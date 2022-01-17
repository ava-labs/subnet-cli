// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package key

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
)

const (
	ewoqPChainAddr    = "P-custom18jma8ppw3nhx5r4ap8clazz0dps7rv5u9xde7p"
	fallbackNetworkID = 999999 // unaffiliated networkID should trigger HRP Fallback
)

func TestNewKeyEwoq(t *testing.T) {
	t.Parallel()

	m, err := New(
		fallbackNetworkID,
		"ewoq",
		WithPrivateKeyEncoded(EwoqPrivateKey),
	)
	if err != nil {
		t.Fatal(err)
	}

	if m.P() != ewoqPChainAddr {
		t.Fatalf("unexpected P-Chain address %q, expected %q", m.P(), ewoqPChainAddr)
	}

	keyPath := filepath.Join(t.TempDir(), "key.pk")
	if err := m.Save(keyPath); err != nil {
		t.Fatal(err)
	}

	m2, err := Load(fallbackNetworkID, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(m.Raw(), m2.Raw()) {
		t.Fatalf("loaded key unexpected %v, expected %v", m2.Raw(), m.Raw())
	}
}

func TestNewKey(t *testing.T) {
	t.Parallel()

	skBytes, err := formatting.Decode(formatting.CB58, rawEwoqPk)
	if err != nil {
		t.Fatal(err)
	}
	factory := &crypto.FactorySECP256K1R{}
	rpk, err := factory.ToPrivateKey(skBytes)
	if err != nil {
		t.Fatal(err)
	}
	ewoqPk, _ := rpk.(*crypto.PrivateKeySECP256K1R)

	rpk2, err := factory.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	privKey2, _ := rpk2.(*crypto.PrivateKeySECP256K1R)

	tt := []struct {
		name   string
		opts   []OpOption
		expErr error
	}{
		{
			name:   "test",
			opts:   nil,
			expErr: nil,
		},
		{
			name: "ewop with WithPrivateKey",
			opts: []OpOption{
				WithPrivateKey(ewoqPk),
			},
			expErr: nil,
		},
		{
			name: "ewop with WithPrivateKeyEncoded",
			opts: []OpOption{
				WithPrivateKeyEncoded(EwoqPrivateKey),
			},
			expErr: nil,
		},
		{
			name: "ewop with WithPrivateKey/WithPrivateKeyEncoded",
			opts: []OpOption{
				WithPrivateKey(ewoqPk),
				WithPrivateKeyEncoded(EwoqPrivateKey),
			},
			expErr: nil,
		},
		{
			name: "ewop with invalid WithPrivateKey",
			opts: []OpOption{
				WithPrivateKey(privKey2),
				WithPrivateKeyEncoded(EwoqPrivateKey),
			},
			expErr: ErrInvalidPrivateKey,
		},
	}
	for i, tv := range tt {
		m, err := New(fallbackNetworkID, tv.name, tv.opts...)
		if !errors.Is(err, tv.expErr) {
			t.Fatalf("#%d: unexpected error %v, expected %v", i, err, tv.expErr)
		}
		if err == nil && m.Name() != tv.name {
			t.Fatalf("#%d: unexpected name %v, expected %v", i, m.Name(), tv.name)
		}
	}
}
