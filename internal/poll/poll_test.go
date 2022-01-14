// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoll(t *testing.T) {
	t.Parallel()

	rootCtx, cancel := context.WithCancel(context.Background())
	cancel()

	pl := New(rootCtx, time.Minute)
	_, err := pl.Poll(context.Background(), nil)
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("unexpected Poll error %v", err)
	}
}
