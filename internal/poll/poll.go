// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package poll defines polling mechanisms.
package poll

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

var ErrAborted = errors.New("aborted")

type Poller interface {
	// Polls until "check" function returns "done=true".
	// If "check" returns a non-empty error, it logs and
	// continues the polling until context is canceled.
	// It returns the duration that it took to complete the check.
	Poll(
		ctx context.Context,
		check func() (done bool, err error),
	) (time.Duration, error)
}

var _ Poller = &poller{}

type poller struct {
	interval time.Duration
}

func New(interval time.Duration) Poller {
	return &poller{
		interval: interval,
	}
}

func (pl *poller) Poll(ctx context.Context, check func() (done bool, err error)) (took time.Duration, err error) {
	start := time.Now()
	zap.L().Info("start polling", zap.String("internal", pl.interval.String()))

	// poll first with no wait
	tc := time.NewTicker(1)
	defer tc.Stop()

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return time.Since(start), ctx.Err()
		case <-tc.C:
			tc.Reset(pl.interval)
		}

		done, err := check()
		if err != nil {
			zap.L().Warn("poll check failed", zap.Error(err))
			continue
		}
		if !done {
			continue
		}

		took := time.Since(start)
		zap.L().Info("poll confirmed", zap.String("took", took.String()))
		return took, nil
	}

	return time.Since(start), ctx.Err()
}
