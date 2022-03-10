// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package key

import (
	"bytes"
	"fmt"
	"sort"

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

const (
	numAddresses = 1024
)

var _ Key = &HardKey{}

type HardKey struct {
	l *ledger.Ledger

	pAddrs       []string
	shortAddrs   []ids.ShortID
	shortAddrMap map[ids.ShortID]uint32
}

func NewHard(networkID uint32) (*HardKey, error) {
	k := &HardKey{}
	var err error
	color.Outf("{{yellow}}connecting to ledger...{{/}}\n")
	k.l, err = ledger.Connect()
	if err != nil {
		color.Outf("{{yellow}}failed to connect to ledger: %v{{/}}\n", err)
		return nil, err
	}

	color.Outf("{{yellow}}deriving address from ledger...{{/}}\n")
	hrp := getHRP(networkID)
	addrs, err := k.l.Addresses(hrp, numAddresses)
	if err != nil {
		color.Outf("{{yellow}}failed to derive address: %v{{/}}\n", err)
		return nil, err
	}

	laddrs := len(addrs)
	k.pAddrs = make([]string, laddrs)
	k.shortAddrs = make([]ids.ShortID, laddrs)
	k.shortAddrMap = map[ids.ShortID]uint32{}
	for i, addr := range addrs {
		k.pAddrs[i], err = formatting.FormatAddress("P", hrp, addr.ShortAddr[:])
		if err != nil {
			return nil, err
		}
		k.shortAddrs[i] = addr.ShortAddr
		k.shortAddrMap[addr.ShortAddr] = uint32(i)
	}

	color.Outf("{{yellow}}derived primary address from ledger: %s{{/}}\n", k.pAddrs[0])
	return k, nil
}

func (h *HardKey) Disconnect() error {
	return h.l.Disconnect()
}

func (h *HardKey) P() []string { return h.pAddrs }

func (h *HardKey) Addresses() []ids.ShortID {
	return h.shortAddrs
}

type innerSortTransferableInputsWithSigners struct {
	ins     []*avax.TransferableInput
	signers [][]ids.ShortID
}

func (ins *innerSortTransferableInputsWithSigners) Less(i, j int) bool {
	iID, iIndex := ins.ins[i].InputSource()
	jID, jIndex := ins.ins[j].InputSource()

	switch bytes.Compare(iID[:], jID[:]) {
	case -1:
		return true
	case 0:
		return iIndex < jIndex
	default:
		return false
	}
}
func (ins *innerSortTransferableInputsWithSigners) Len() int { return len(ins.ins) }
func (ins *innerSortTransferableInputsWithSigners) Swap(i, j int) {
	ins.ins[j], ins.ins[i] = ins.ins[i], ins.ins[j]
	ins.signers[j], ins.signers[i] = ins.signers[i], ins.signers[j]
}

// SortTransferableInputsWithSigners sorts the inputs and signers based on the
// input's utxo ID
func SortTransferableInputsWithSigners(ins []*avax.TransferableInput, signers [][]ids.ShortID) {
	sort.Sort(&innerSortTransferableInputsWithSigners{ins: ins, signers: signers})
}

func (h *HardKey) Spends(outputs []*avax.UTXO, opts ...OpOption) (
	totalBalanceToSpend uint64,
	inputs []*avax.TransferableInput,
	signers [][]ids.ShortID,
) {
	ret := &Op{}
	ret.applyOpts(opts)

	for _, out := range outputs {
		input, txsigners, err := h.spend(out, ret.time)
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
		signers = append(signers, txsigners)
		if ret.targetAmount > 0 &&
			totalBalanceToSpend > ret.targetAmount+ret.feeDeduct {
			break
		}
	}
	SortTransferableInputsWithSigners(inputs, signers)
	return totalBalanceToSpend, inputs, signers
}

func (h *HardKey) spend(output *avax.UTXO, time uint64) (
	input avax.TransferableIn,
	signers []ids.ShortID,
	err error,
) {
	// "time" is used to check whether the key owner
	// is still within the lock time (thus can't spend).
	inputf, signers, err := h.lspend(output.Out, time)
	if err != nil {
		return nil, nil, err
	}
	var ok bool
	input, ok = inputf.(avax.TransferableIn)
	if !ok {
		return nil, nil, ErrInvalidType
	}
	return input, signers, nil
}

// Spend attempts to create an input.
func (h *HardKey) lspend(out verify.Verifiable, time uint64) (verify.Verifiable, []ids.ShortID, error) {
	switch out := out.(type) {
	case *secp256k1fx.MintOutput:
		if sigIndices, signers, able := h.match(&out.OutputOwners, time); able {
			return &secp256k1fx.Input{
				SigIndices: sigIndices,
			}, signers, nil
		}
		return nil, nil, ErrCantSpend
	case *secp256k1fx.TransferOutput:
		if sigIndices, signers, able := h.match(&out.OutputOwners, time); able {
			return &secp256k1fx.TransferInput{
				Amt: out.Amt,
				Input: secp256k1fx.Input{
					SigIndices: sigIndices,
				},
			}, signers, nil
		}
		return nil, nil, ErrCantSpend
	}
	return nil, nil, fmt.Errorf("can't spend UTXO because it is unexpected type %T", out)
}

// Match attempts to match a list of addresses up to the provided threshold.
func (h *HardKey) match(owners *secp256k1fx.OutputOwners, time uint64) ([]uint32, []ids.ShortID, bool) {
	if time < owners.Locktime {
		return nil, nil, false
	}
	sigs := make([]uint32, 0, owners.Threshold)
	signers := make([]ids.ShortID, 0, owners.Threshold)
	for i := uint32(0); i < uint32(len(owners.Addrs)) && uint32(len(sigs)) < owners.Threshold; i++ {
		if _, ok := h.shortAddrMap[owners.Addrs[i]]; ok {
			sigs = append(sigs, i)
			signers = append(signers, owners.Addrs[i])
		}
	}
	return sigs, signers, uint32(len(sigs)) == owners.Threshold
}

// Sign transaction with the ledger private key
//
// This is a slightly modified version of *platformvm.Tx.Sign().
func (h *HardKey) Sign(pTx *platformvm.Tx, signers [][]ids.ShortID) error {
	unsignedBytes, err := codec.PCodecManager.Marshal(platformvm.CodecVersion, &pTx.UnsignedTx)
	if err != nil {
		return fmt.Errorf("couldn't marshal UnsignedTx: %w", err)
	}
	hash := hashing.ComputeHash256(unsignedBytes)

	// Generate signature
	uniqueSigners := map[uint32]struct{}{}
	for _, inputSigners := range signers {
		for _, signer := range inputSigners {
			if v, ok := h.shortAddrMap[signer]; ok {
				uniqueSigners[v] = struct{}{}
			} else {
				// Should never happen
				return ErrCantSpend
			}
		}
	}
	indices := make([]uint32, len(uniqueSigners))
	for idx := range uniqueSigners {
		indices = append(indices, idx)
	}
	sigs, err := h.l.SignHash(hash, indices)
	if err != nil {
		return fmt.Errorf("problem generating signatures: %w", err)
	}
	sigMap := map[ids.ShortID][]byte{}
	for i, idx := range indices {
		sigMap[h.shortAddrs[idx]] = sigs[i]
	}

	// Add credentials to transaction
	for _, inputSigners := range signers {
		cred := &secp256k1fx.Credential{
			Sigs: make([][crypto.SECP256K1RSigLen]byte, len(inputSigners)),
		}
		for i, signer := range inputSigners {
			copy(cred.Sigs[i][:], sigMap[signer])
		}
		pTx.Creds = append(pTx.Creds, cred)
	}

	// Create signed tx bytes
	signedBytes, err := codec.PCodecManager.Marshal(platformvm.CodecVersion, pTx)
	if err != nil {
		return fmt.Errorf("couldn't marshal ProposalTx: %w", err)
	}
	pTx.Initialize(unsignedBytes, signedBytes)
	return nil
}
