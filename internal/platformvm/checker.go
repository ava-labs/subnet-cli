// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

// TO BE MOVED TO "github.com/ava-labs/avalanchego/vms/platformvm"

import (
	"context"
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	pstatus "github.com/ava-labs/avalanchego/vms/platformvm/status"
	"github.com/ava-labs/subnet-cli/internal/poll"
	"go.uber.org/zap"
)

var (
	ErrInvalidCheckerOpOption = errors.New("invalid checker OpOption")
	ErrEmptyID                = errors.New("empty ID")
	ErrAbortedDropped         = errors.New("aborted/dropped")
)

type Checker interface {
	PollTx(ctx context.Context, txID ids.ID, s pstatus.Status) (time.Duration, error)
	PollSubnet(ctx context.Context, subnetID ids.ID) (time.Duration, error)
	PollBlockchain(ctx context.Context, opts ...OpOption) (time.Duration, error)
}

var _ Checker = &checker{}

type checker struct {
	poller poll.Poller
	cli    platformvm.Client
}

func NewChecker(poller poll.Poller, cli platformvm.Client) Checker {
	return &checker{
		poller: poller,
		cli:    cli,
	}
}

func (c *checker) PollTx(ctx context.Context, txID ids.ID, s pstatus.Status) (time.Duration, error) {
	zap.L().Info("polling P-Chain tx",
		zap.String("txId", txID.String()),
		zap.String("expectedStatus", s.String()),
	)
	return c.poller.Poll(ctx, func() (done bool, err error) {
		status, err := c.cli.GetTxStatus(ctx, txID, true)
		if err != nil {
			return false, err
		}
		zap.L().Debug("tx",
			zap.String("status", status.Status.String()),
			zap.String("reason", status.Reason),
		)
		if s == pstatus.Committed &&
			(status.Status == pstatus.Aborted || status.Status == pstatus.Dropped) {
			return true, ErrAbortedDropped
		}
		return status.Status == s, nil
	})
}

func (c *checker) PollSubnet(ctx context.Context, subnetID ids.ID) (took time.Duration, err error) {
	if subnetID == ids.Empty {
		return took, ErrEmptyID
	}

	zap.L().Info("polling subnet",
		zap.String("subnetId", subnetID.String()),
	)
	took, err = c.PollTx(ctx, subnetID, pstatus.Committed)
	if err != nil {
		return took, err
	}
	prev := took
	took, err = c.findSubnet(ctx, subnetID)
	took += prev
	return took, err
}

func (c *checker) findSubnet(ctx context.Context, subnetID ids.ID) (took time.Duration, err error) {
	zap.L().Info("finding subnets",
		zap.String("subnetId", subnetID.String()),
	)
	took, err = c.poller.Poll(ctx, func() (done bool, err error) {
		ss, err := c.cli.GetSubnets(ctx, []ids.ID{subnetID})
		if err != nil {
			return false, err
		}
		if len(ss) != 1 {
			return false, nil
		}
		return ss[0].ID == subnetID, nil
	})
	return took, err
}

func (c *checker) PollBlockchain(ctx context.Context, opts ...OpOption) (took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	if ret.subnetID == ids.Empty &&
		ret.blockchainID == ids.Empty {
		return took, ErrEmptyID
	}

	if ret.blockchainID == ids.Empty {
		ret.blockchainID, took, err = c.findBlockchain(ctx, ret.subnetID)
		if err != nil {
			return took, err
		}
	}
	if ret.blockchainID == ids.Empty {
		return took, ErrEmptyID
	}

	if ret.checkBlockchainBootstrapped && ret.info == nil {
		return took, ErrInvalidCheckerOpOption
	}

	zap.L().Info("polling blockchain",
		zap.String("blockchainId", ret.blockchainID.String()),
		zap.String("expectedBlockchainStatus", ret.blockchainStatus.String()),
	)

	prev := took
	took, err = c.PollTx(ctx, ret.blockchainID, pstatus.Committed)
	took += prev
	if err != nil {
		return took, err
	}

	statusPolled := false
	prev = took
	took, err = c.poller.Poll(ctx, func() (done bool, err error) {
		if !statusPolled {
			status, err := c.cli.GetBlockchainStatus(ctx, ret.blockchainID.String())
			if err != nil {
				return false, err
			}
			if status != ret.blockchainStatus {
				zap.L().Info("waiting for blockchain status",
					zap.String("current", status.String()),
				)
				return false, nil
			}
			statusPolled = true
			if !ret.checkBlockchainBootstrapped {
				return true, nil
			}
		}

		bootstrapped, err := ret.info.IsBootstrapped(ctx, ret.blockchainID.String())
		if err != nil {
			return false, err
		}
		if !bootstrapped {
			zap.L().Debug("blockchain not bootstrapped yet; retrying")
			return false, nil
		}
		return true, nil
	})
	took += prev
	return took, err
}

func (c *checker) findBlockchain(ctx context.Context, subnetID ids.ID) (bchID ids.ID, took time.Duration, err error) {
	zap.L().Info("finding blockchains",
		zap.String("subnetId", subnetID.String()),
	)
	took, err = c.poller.Poll(ctx, func() (done bool, err error) {
		bcs, err := c.cli.GetBlockchains(ctx)
		if err != nil {
			return false, err
		}
		bchID = ids.Empty
		for _, blockchain := range bcs {
			if blockchain.SubnetID == subnetID {
				bchID = blockchain.ID
				break
			}
		}
		return bchID == ids.Empty, nil
	})
	return bchID, took, err
}

type Op struct {
	subnetID     ids.ID
	blockchainID ids.ID

	blockchainStatus pstatus.BlockchainStatus

	info                        info.Client
	checkBlockchainBootstrapped bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithSubnetID(subnetID ids.ID) OpOption {
	return func(op *Op) {
		op.subnetID = subnetID
	}
}

func WithBlockchainID(bch ids.ID) OpOption {
	return func(op *Op) {
		op.blockchainID = bch
	}
}

func WithBlockchainStatus(s pstatus.BlockchainStatus) OpOption {
	return func(op *Op) {
		op.blockchainStatus = s
	}
}

// TODO: avalanchego "GetBlockchainStatusReply" should have "Bootstrapped".
// e.g., "service.vm.Chains.IsBootstrapped" in "GetBlockchainStatus".
func WithCheckBlockchainBootstrapped(info info.Client) OpOption {
	return func(op *Op) {
		op.info = info
		op.checkBlockchainBootstrapped = true
	}
}
