// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	api_keystore "github.com/ava-labs/avalanchego/api/keystore"
)

type KeyStore interface {
	Client() api_keystore.Client
}

type keyStore struct {
	cli api_keystore.Client
	cfg Config
}

func newKeyStore(cfg Config) *keyStore {
	cli := api_keystore.NewClient(cfg.URI, cfg.RequestTimeout)
	return &keyStore{
		cli: cli,
		cfg: cfg,
	}
}

func (k *keyStore) Client() api_keystore.Client { return k.cli }
