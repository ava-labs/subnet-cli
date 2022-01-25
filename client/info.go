// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	api_info "github.com/ava-labs/avalanchego/api/info"
)

type Info interface {
	Client() api_info.Client
}

type info struct {
	cli api_info.Client
	cfg Config
}

func newInfo(cfg Config) *info {
	// "NewClient" already appends "/ext/info"
	// e.g., https://api.avax-test.network
	// ref. https://docs.avax.network/build/avalanchego-apis/info
	uri := cfg.u.Scheme + "://" + cfg.u.Host
	cli := api_info.NewClient(uri, cfg.RequestTimeout)
	return &info{
		cli: cli,
		cfg: cfg,
	}
}

func (i *info) Client() api_info.Client { return i.cli }
