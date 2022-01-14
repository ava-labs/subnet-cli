// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package client implements client.
package client

import (
	"context"
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	avago_constants "github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/subnet-cli/internal/poll"
	"go.uber.org/zap"
)

var (
	ErrEmptyID               = errors.New("empty ID")
	ErrEmptyURI              = errors.New("empty URI")
	ErrInvalidInterval       = errors.New("invalid interval")
	ErrInvalidRequestTimeout = errors.New("invalid request timeout")
)

type Config struct {
	RootCtx context.Context

	URI string

	// Network ID set in the genesis.
	NetworkID uint32

	// avax asset ID
	AssetID  ids.ID
	XChainID ids.ID
	PChainID ids.ID

	PollInterval   time.Duration
	RequestTimeout time.Duration
}

var _ Client = &client{}

type Client interface {
	Config() Config

	Info() Info
	KeyStore() KeyStore

	P() P
}

type client struct {
	cfg Config
	i   *info
	k   *keyStore
	p   *p
}

func New(cfg Config) (Client, error) {
	if cfg.URI == "" {
		return nil, ErrEmptyURI
	}
	if cfg.PollInterval == time.Duration(0) {
		return nil, ErrInvalidInterval
	}
	if cfg.RequestTimeout == time.Duration(0) {
		return nil, ErrInvalidRequestTimeout
	}

	ii := newInfo(cfg)
	kk := newKeyStore(cfg)

	if cfg.PChainID == ids.Empty {
		cfg.PChainID = avago_constants.PlatformChainID
	}
	if cfg.AssetID == ids.Empty {
		zap.L().Info("fetching X-Chain id")
		xChainID, err := ii.cli.GetBlockchainID("X")
		if err != nil {
			return nil, err
		}
		cfg.XChainID = xChainID

		zap.L().Info("fetching AVAX asset id")
		xc := avm.NewClient(cfg.URI, cfg.XChainID.String(), cfg.RequestTimeout)
		avaxDesc, err := xc.GetAssetDescription("AVAX")
		if err != nil {
			return nil, err
		}
		cfg.AssetID = avaxDesc.AssetID
	}
	if cfg.AssetID == ids.Empty {
		return nil, ErrEmptyID
	}

	pl := poll.New(cfg.RootCtx, cfg.PollInterval)
	pp := newP(cfg, ii.Client(), pl)

	return &client{
		cfg: cfg,
		i:   ii,
		k:   kk,
		p:   pp,
	}, nil
}

func (cc *client) Config() Config { return cc.cfg }

func (cc *client) Info() Info         { return cc.i }
func (cc *client) KeyStore() KeyStore { return cc.k }

func (cc *client) P() P { return cc.p }
