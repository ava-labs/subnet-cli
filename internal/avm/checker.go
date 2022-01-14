// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

// TO BE MOVED TO "github.com/ava-labs/avalanchego/vms/avm"

import (
	"context"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/subnet-cli/internal/poll"
	"github.com/ava-labs/subnet-cli/pkg/color"
)

type Checker interface {
	// Blocks until the tx status evaluates to "s".
	PollTx(ctx context.Context, txID ids.ID, s choices.Status) (time.Duration, error)
	PollBalance(ctx context.Context, addr string, balance uint64, opts ...OpOption) (time.Duration, error)
}

var _ Checker = &checker{}

type checker struct {
	poller poll.Poller
	cli    avm.Client
}

func NewChecker(poller poll.Poller, cli avm.Client) Checker {
	return &checker{
		poller: poller,
		cli:    cli,
	}
}

func (c *checker) PollTx(ctx context.Context, txID ids.ID, s choices.Status) (time.Duration, error) {
	color.Outf("{{blue}}polling X-Chain tx %s for %q{{/}}\n", txID, s)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		status, err := c.cli.GetTxStatus(txID)
		if err != nil {
			return false, err
		}
		return status == s, nil
	})
}

func (c *checker) PollBalance(
	ctx context.Context,
	addr string,
	balance uint64,
	opts ...OpOption,
) (time.Duration, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	color.Outf("{{blue}}polling X-Chain balance %q for %d $AVAX{{/}}\n", addr, balance)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		bal, err := c.cli.GetBalance(addr, "AVAX", false)
		if err != nil {
			return false, err
		}
		if ret.balanceEqual {
			return uint64(bal.Balance) == balance, nil
		}
		return uint64(bal.Balance) >= balance, nil
	})
}

type Op struct {
	balanceEqual bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

// To require poll to return if and only if
// the balance is the exact match.
func WithBalanceEqual() OpOption {
	return func(op *Op) {
		op.balanceEqual = true
	}
}
