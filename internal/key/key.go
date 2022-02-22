// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package key implements key manager and helper functions.
package key

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"strings"

	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"go.uber.org/zap"
)

var (
	ErrInvalidType = errors.New("invalid type")

	// ErrInvalidPrivateKey is returned when specified privates are invalid.
	ErrInvalidPrivateKey         = errors.New("invalid private key")
	ErrInvalidPrivateKeyLen      = errors.New("invalid private key length (expect 64 bytes in hex)")
	ErrInvalidPrivateKeyEnding   = errors.New("invalid private key ending")
	ErrInvalidPrivateKeyEncoding = errors.New("invalid private key encoding")
)

// Key defines methods for key manager interface.
// TODO: separate addresser/Spender from Key
type Key interface {
	Addresser
	Spender

	// Returns the name of the key.
	Name() string
	// Returns the private key.
	Key() *crypto.PrivateKeySECP256K1R
	// Returns the private key in raw bytes.
	Raw() []byte
	// Returns the private key encoded in CB58 and "PrivateKey-" prefix.
	Encode() string
	// Saves the private key to disk with hex encoding.
	Save(p string) error
}

type Addresser interface {
	// P returns the P-Chain address.
	P() string
}

type Spender interface {
	// Spend attempts to spend all specified UTXOs (outputs)
	// and returns the new UTXO inputs.
	// If target amount is specified, it only uses the
	// outputs until the total spending is below the target
	// amount.
	Spends(outputs []*avax.UTXO, opts ...OpOption) (
		totalBalanceToSpend uint64,
		inputs []*avax.TransferableInput,
		inputSigners [][]*crypto.PrivateKeySECP256K1R,
	)
}

var _ Key = &manager{}

type manager struct {
	hrp  string
	name string

	privKey        *crypto.PrivateKeySECP256K1R
	privKeyRaw     []byte
	privKeyEncoded string

	pAddr string

	keyChain *secp256k1fx.Keychain
}

const (
	privKeyEncPfx = "PrivateKey-"
	privKeySize   = 64

	rawEwoqPk      = "ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN"
	EwoqPrivateKey = "PrivateKey-" + rawEwoqPk
)

var keyFactory = new(crypto.FactorySECP256K1R)

func New(networkID uint32, name string, opts ...OpOption) (Key, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	// set via "WithPrivateKeyEncoded"
	if len(ret.privKeyEncoded) > 0 {
		privKey, err := decodePrivateKey(ret.privKeyEncoded)
		if err != nil {
			return nil, err
		}
		// to not overwrite
		if ret.privKey != nil &&
			!bytes.Equal(ret.privKey.Bytes(), privKey.Bytes()) {
			return nil, ErrInvalidPrivateKey
		}
		ret.privKey = privKey
	}

	// generate a new one
	if ret.privKey == nil {
		rpk, err := keyFactory.NewPrivateKey()
		if err != nil {
			return nil, err
		}
		var ok bool
		ret.privKey, ok = rpk.(*crypto.PrivateKeySECP256K1R)
		if !ok {
			return nil, ErrInvalidType
		}
	}

	privKey := ret.privKey
	privKeyEncoded, err := encodePrivateKey(ret.privKey)
	if err != nil {
		return nil, err
	}

	// double-check encoding is consistent
	if ret.privKeyEncoded != "" &&
		ret.privKeyEncoded != privKeyEncoded {
		return nil, ErrInvalidPrivateKeyEncoding
	}

	keyChain := secp256k1fx.NewKeychain()
	keyChain.Add(privKey)

	m := &manager{
		name: name,

		privKey:        privKey,
		privKeyRaw:     privKey.Bytes(),
		privKeyEncoded: privKeyEncoded,

		keyChain: keyChain,
	}

	// Parse HRP to create valid address
	// TODO: use this with address
	switch networkID {
	case constants.LocalID:
		m.hrp = constants.LocalHRP
	case constants.FujiID:
		m.hrp = constants.FujiHRP
	case constants.MainnetID:
		m.hrp = constants.MainnetHRP
	default:
		m.hrp = constants.FallbackHRP
	}

	if err := m.updateAddr(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *manager) Name() string {
	return m.name
}

func (m *manager) Key() *crypto.PrivateKeySECP256K1R {
	return m.privKey
}

func (m *manager) Raw() []byte {
	return m.privKeyRaw
}

func (m *manager) Encode() string {
	return m.privKeyEncoded
}

func (m *manager) P() string { return m.pAddr }

func (m *manager) Spends(outputs []*avax.UTXO, opts ...OpOption) (
	totalBalanceToSpend uint64,
	inputs []*avax.TransferableInput,
	inputSigners [][]*crypto.PrivateKeySECP256K1R,
) {
	ret := &Op{}
	ret.applyOpts(opts)

	for _, out := range outputs {
		input, signers, err := m.spend(out, ret.time)
		if err != nil {
			zap.L().Warn("cannot spend with current key", zap.Error(err))
			continue
		}
		totalBalanceToSpend += input.Amount()
		inputs = append(inputs, &avax.TransferableInput{
			UTXOID: out.UTXOID,
			Asset:  out.Asset,
			In:     input,
		})
		inputSigners = append(inputSigners, signers)
		if ret.targetAmount > 0 &&
			totalBalanceToSpend > ret.targetAmount+ret.feeDeduct {
			break
		}
	}
	avax.SortTransferableInputsWithSigners(inputs, inputSigners)

	return totalBalanceToSpend, inputs, inputSigners
}

func (m *manager) spend(output *avax.UTXO, time uint64) (
	input avax.TransferableIn,
	inputSigners []*crypto.PrivateKeySECP256K1R,
	err error,
) {
	// "time" is used to check whether the key owner
	// is still within the lock time (thus can't spend).
	inputf, inputSigners, err := m.keyChain.Spend(output.Out, time)
	if err != nil {
		return nil, nil, err
	}
	var ok bool
	input, ok = inputf.(avax.TransferableIn)
	if !ok {
		return nil, nil, ErrInvalidType
	}
	return input, inputSigners, nil
}

const fsModeWrite = 0o600

func (m *manager) Save(p string) error {
	k := hex.EncodeToString(m.privKeyRaw)
	return ioutil.WriteFile(p, []byte(k), fsModeWrite)
}

// Loads the private key from disk and creates the corresponding manager.
func Load(networkID uint32, keyPath string) (Key, error) {
	kb, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// in case, it's already encoded
	k, err := New(networkID, keyPath, WithPrivateKeyEncoded(string(kb)))
	if err == nil {
		return k, nil
	}

	r := bufio.NewReader(bytes.NewBuffer(kb))
	buf := make([]byte, privKeySize)
	n, err := readASCII(buf, r)
	if err != nil {
		return nil, err
	}
	if n != len(buf) {
		return nil, ErrInvalidPrivateKeyLen
	}
	if err := checkKeyFileEnd(r); err != nil {
		return nil, err
	}

	skBytes, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	rpk, err := keyFactory.ToPrivateKey(skBytes)
	if err != nil {
		return nil, err
	}
	privKey, ok := rpk.(*crypto.PrivateKeySECP256K1R)
	if !ok {
		return nil, ErrInvalidType
	}

	return New(networkID, keyPath, WithPrivateKey(privKey))
}

// readASCII reads into 'buf', stopping when the buffer is full or
// when a non-printable control character is encountered.
func readASCII(buf []byte, r io.ByteReader) (n int, err error) {
	for ; n < len(buf); n++ {
		buf[n], err = r.ReadByte()
		switch {
		case errors.Is(err, io.EOF) || buf[n] < '!':
			return n, nil
		case err != nil:
			return n, err
		}
	}
	return n, nil
}

const fileEndLimit = 1

// checkKeyFileEnd skips over additional newlines at the end of a key file.
func checkKeyFileEnd(r io.ByteReader) error {
	for idx := 0; ; idx++ {
		b, err := r.ReadByte()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		case b != '\n' && b != '\r':
			return ErrInvalidPrivateKeyEnding
		case idx > fileEndLimit:
			return ErrInvalidPrivateKeyLen
		}
	}
}

func encodePrivateKey(pk *crypto.PrivateKeySECP256K1R) (string, error) {
	privKeyRaw := pk.Bytes()
	enc, err := formatting.EncodeWithChecksum(formatting.CB58, privKeyRaw)
	if err != nil {
		return "", err
	}
	return privKeyEncPfx + enc, nil
}

func decodePrivateKey(enc string) (*crypto.PrivateKeySECP256K1R, error) {
	rawPk := strings.Replace(enc, privKeyEncPfx, "", 1)
	skBytes, err := formatting.Decode(formatting.CB58, rawPk)
	if err != nil {
		return nil, err
	}
	rpk, err := keyFactory.ToPrivateKey(skBytes)
	if err != nil {
		return nil, err
	}
	privKey, ok := rpk.(*crypto.PrivateKeySECP256K1R)
	if !ok {
		return nil, ErrInvalidType
	}
	return privKey, nil
}

func (m *manager) updateAddr() (err error) {
	pubBytes := m.privKey.PublicKey().Address().Bytes()
	m.pAddr, err = formatting.FormatAddress("P", m.hrp, pubBytes)
	if err != nil {
		return err
	}
	return nil
}

type Op struct {
	privKey        *crypto.PrivateKeySECP256K1R
	privKeyEncoded string

	time         uint64
	targetAmount uint64
	feeDeduct    uint64
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

// To create a new key manager with a pre-loaded private key.
func WithPrivateKey(privKey *crypto.PrivateKeySECP256K1R) OpOption {
	return func(op *Op) {
		op.privKey = privKey
	}
}

// To create a new key manager with a pre-defined private key.
func WithPrivateKeyEncoded(privKey string) OpOption {
	return func(op *Op) {
		op.privKeyEncoded = privKey
	}
}

func WithTime(t uint64) OpOption {
	return func(op *Op) {
		op.time = t
	}
}

func WithTargetAmount(ta uint64) OpOption {
	return func(op *Op) {
		op.targetAmount = ta
	}
}

// To deduct transfer fee from total spend (output).
// e.g., "units.MilliAvax" for X/P-Chain transfer.
func WithFeeDeduct(fee uint64) OpOption {
	return func(op *Op) {
		op.feeDeduct = fee
	}
}
