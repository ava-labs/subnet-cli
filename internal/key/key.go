// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package key implements key manager and helper functions.
package key

import (
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

// Key defines methods for key manager interface.
type Key interface {
	// P returns the P-Chain address.
	P() string

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
	privKey        *crypto.PrivateKeySECP256K1R
	privKeyEncoded string

	Time         uint64
	TargetAmount uint64
	FeeDeduct    uint64
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) {
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
		op.Time = t
	}
}

func WithTargetAmount(ta uint64) OpOption {
	return func(op *Op) {
		op.TargetAmount = ta
	}
}

// To deduct transfer fee from total spend (output).
// e.g., "units.MilliAvax" for X/P-Chain transfer.
func WithFeeDeduct(fee uint64) OpOption {
	return func(op *Op) {
		op.FeeDeduct = fee
	}
}
