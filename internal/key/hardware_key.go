// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package key implements key manager and helper functions.
package key

import (
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

var _ Key = &hardwareKey{}

type hardwareKey struct {
	hrp string
}

func NewHardwareKey(networkID uint32, opts ...OpOption) (Key, error) {
	return nil, nil
}

func (k *hardwareKey) P() string {
	return ""
}

func (k *hardwareKey) Spends(outputs []*avax.UTXO, opts ...OpOption) (
	totalBalanceToSpend uint64,
	inputs []*avax.TransferableInput,
) {
	return 0, nil
}
