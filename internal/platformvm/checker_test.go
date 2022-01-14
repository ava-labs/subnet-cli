// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"context"
	"errors"
	"testing"
)

func TestChecker(t *testing.T) {
	t.Parallel()

	ck := NewChecker(nil, nil)
	_, err := ck.PollBlockchain(context.Background())
	if !errors.Is(err, ErrEmptyID) {
		t.Fatalf("unexpected error %v, expected %v", err, ErrEmptyID)
	}
}
