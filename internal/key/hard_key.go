// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package key

import (
	ledger "github.com/ava-labs/avalanche-ledger-go"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm"

	"github.com/ava-labs/subnet-cli/pkg/color"
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
	return 0, nil
}

func (h *HardKey) Sign(pTx *platformvm.Tx, sigs int) error {
	return nil
}
