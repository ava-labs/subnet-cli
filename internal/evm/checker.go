// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

// TO BE MOVED TO "github.com/ava-labs/avalanchego/vms/evm"

import (
	"context"
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	eth_client "github.com/ava-labs/coreth/ethclient"
	evm_client "github.com/ava-labs/coreth/plugin/evm"
	"github.com/ava-labs/subnet-cli/internal/poll"
	"github.com/ava-labs/subnet-cli/pkg/color"
	eth_common "github.com/ethereum/go-ethereum/common"
)

type Checker interface {
	PollAtomicTx(ctx context.Context, txID ids.ID, expected evm_client.Status) (time.Duration, error)
	PollNonce(ctx context.Context, account eth_common.Address, broadcastNonce uint64) (time.Duration, error)
	PollBalance(ctx context.Context, account eth_common.Address, balance uint64, opts ...OpOption) (time.Duration, error)
}

var ErrTxDropped = errors.New("tx dropped")

var _ Checker = &checker{}

type checker struct {
	poller poll.Poller
	ethCli *eth_client.Client
	evmCli *evm_client.Client
}

func NewChecker(poller poll.Poller, ethCli *eth_client.Client, evmCli *evm_client.Client) Checker {
	return &checker{
		poller: poller,
		ethCli: ethCli,
		evmCli: evmCli,
	}
}

func (c *checker) PollAtomicTx(ctx context.Context, txID ids.ID, expected evm_client.Status) (time.Duration, error) {
	color.Outf("{{blue}}polling C-Chain atomic tx %s for %q{{/}}\n", txID, expected)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		cur, err := c.evmCli.GetAtomicTxStatus(txID)
		if err != nil {
			return false, err
		}
		if expected == evm_client.Accepted && cur == evm_client.Dropped {
			return true, ErrTxDropped
		}
		return cur == expected, nil
	})
}

func (c *checker) PollNonce(ctx context.Context, account eth_common.Address, broadcastNonce uint64) (time.Duration, error) {
	color.Outf("{{blue}}polling C-Chain account with nonce %d{{/}}\n", broadcastNonce)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		cur, err := c.ethCli.NonceAt(ctx, account, nil)
		if err != nil {
			return false, err
		}
		return cur >= broadcastNonce, nil
	})
}

func (c *checker) PollBalance(
	ctx context.Context,
	account eth_common.Address,
	balance uint64,
	opts ...OpOption,
) (time.Duration, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	color.Outf("{{blue}}polling P-Chain balance %q for %d $AVAX{{/}}\n", account, balance)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		bal, err := c.ethCli.BalanceAt(ctx, account, nil)
		if err != nil {
			return false, err
		}
		if ret.balanceEqual {
			return bal.Uint64() == balance, nil
		}
		return bal.Uint64() >= balance, nil
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
