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

	pl := New(time.Minute)

	rootCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pl.Poll(rootCtx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected Poll error %v", err)
	}
}
