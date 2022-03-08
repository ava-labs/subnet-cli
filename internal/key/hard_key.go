// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package key

import (
	"fmt"

	"github.com/ava-labs/subnet-cli/internal/codec"
	"github.com/ava-labs/subnet-cli/pkg/color"

	ledger "github.com/ava-labs/avalanche-ledger-go"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"go.uber.org/zap"
)

var _ Key = &HardKey{}

type HardKey struct {
	l *ledger.Ledger

	hrp       string
	shortAddr ids.ShortID
	pAddr     string
}

func NewHard(networkID uint32) (*HardKey, error) {
	k := &HardKey{}
	var err error
	color.Outf("{{yellow}}connecting to ledger...{{/}}\n")
	k.l, err = ledger.Connect()
	if err != nil {
		return nil, err
	}

	color.Outf("{{yellow}}deriving address from ledger...{{/}}\n")
	k.hrp = getHRP(networkID)
	_, k.shortAddr, err = k.l.Address(k.hrp, 0, 0)
	if err != nil {
		return nil, err
	}

	k.pAddr, err = formatting.FormatAddress("P", k.hrp, k.shortAddr[:])
	if err != nil {
		return nil, err
	}
	color.Outf("{{yellow}}derived address from ledger: %s{{/}}\n", k.pAddr)

	return k, nil
}

func (h *HardKey) P() string { return h.pAddr }

func (h *HardKey) Address() ids.ShortID {
	return h.shortAddr
}

func (h *HardKey) Spends(outputs []*avax.UTXO, opts ...OpOption) (
	totalBalanceToSpend uint64,
	inputs []*avax.TransferableInput,
) {
	ret := &Op{}
	ret.applyOpts(opts)

	for _, out := range outputs {
		input, err := h.spend(out, ret.time)
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
		if ret.targetAmount > 0 &&
			totalBalanceToSpend > ret.targetAmount+ret.feeDeduct {
			break
		}
	}
	avax.SortTransferableInputs(inputs)

	return totalBalanceToSpend, inputs
}

func (h *HardKey) spend(output *avax.UTXO, time uint64) (
	input avax.TransferableIn,
	err error,
) {
	// "time" is used to check whether the key owner
	// is still within the lock time (thus can't spend).
	inputf, err := h.lspend(output.Out, time)
	if err != nil {
		return nil, err
	}
	var ok bool
	input, ok = inputf.(avax.TransferableIn)
	if !ok {
		return nil, ErrInvalidType
	}
	return input, nil
}

// Spend attempts to create an input
func (h *HardKey) lspend(out verify.Verifiable, time uint64) (verify.Verifiable, error) {
	switch out := out.(type) {
	case *secp256k1fx.MintOutput:
		if sigIndices, able := h.match(&out.OutputOwners, time); able {
			return &secp256k1fx.Input{
				SigIndices: sigIndices,
			}, nil
		}
		return nil, ErrCantSpend
	case *secp256k1fx.TransferOutput:
		if sigIndices, able := h.match(&out.OutputOwners, time); able {
			return &secp256k1fx.TransferInput{
				Amt: out.Amt,
				Input: secp256k1fx.Input{
					SigIndices: sigIndices,
				},
			}, nil
		}
		return nil, ErrCantSpend
	}
	return nil, fmt.Errorf("can't spend UTXO because it is unexpected type %T", out)
}

// Match attempts to match a list of addresses up to the provided threshold
func (h *HardKey) match(owners *secp256k1fx.OutputOwners, time uint64) ([]uint32, bool) {
	if time < owners.Locktime {
		return nil, false
	}
	sigs := make([]uint32, 0, owners.Threshold)
	for i := uint32(0); i < uint32(len(owners.Addrs)) && uint32(len(sigs)) < owners.Threshold; i++ {
		if owners.Addrs[i] == h.shortAddr {
			sigs = append(sigs, i)
		}
	}
	return sigs, uint32(len(sigs)) == owners.Threshold
}

// Sign transaction with the ledger private key
//
// This is a slightly modified version of *platformvm.Tx.Sign().
func (h *HardKey) Sign(pTx *platformvm.Tx, sigs int) error {
	unsignedBytes, err := codec.PCodecManager.Marshal(platformvm.CodecVersion, &pTx.UnsignedTx)
	if err != nil {
		return fmt.Errorf("couldn't marshal UnsignedTx: %w", err)
	}

	// Generate signature
	hash := hashing.ComputeHash256(unsignedBytes)
	cred := &secp256k1fx.Credential{
		Sigs: make([][crypto.SECP256K1RSigLen]byte, 1),
	}
	sig, err := h.l.SignHash(hash, [][]uint32{{0, 0}})
	if err != nil {
		return fmt.Errorf("problem generating credential: %w", err)
	}

	// Copy signature required times
	copy(cred.Sigs[0][:], sig[0])
	for i := 0; i < sigs; i++ {
		pTx.Creds = append(pTx.Creds, cred) // Attach credential
	}

	signedBytes, err := codec.PCodecManager.Marshal(platformvm.CodecVersion, pTx)
	if err != nil {
		return fmt.Errorf("couldn't marshal ProposalTx: %w", err)
	}
	pTx.Initialize(unsignedBytes, signedBytes)
	return nil
}
