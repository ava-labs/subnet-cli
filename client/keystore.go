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
	// "NewClient" already appends "/ext/keystore"
	// e.g., https://api.avax-test.network
	// ref. https://docs.avax.network/build/avalanchego-apis/keystore
	uri := cfg.u.Scheme + "://" + cfg.u.Host
	cli := api_keystore.NewClient(uri, cfg.RequestTimeout)
	return &keyStore{
		cli: cli,
		cfg: cfg,
	}
}

func (k *keyStore) Client() api_keystore.Client { return k.cli }
