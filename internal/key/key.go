// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package key implements key manager and helper functions.
package key

import (
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

// Key defines methods for key manager interface.
type Key interface {
	Addresser
	Spender
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
	)
}

type Op struct {
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
