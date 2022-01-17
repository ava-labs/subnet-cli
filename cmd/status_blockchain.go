// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package implements "status" sub-commands.
package cmd

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/subnet-cli/internal/client"
	internal_platformvm "github.com/ava-labs/subnet-cli/internal/platformvm"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	checkBootstrapped bool
)

func newStatusBlockchainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blockchain [BLOCKCHAIN ID]",
		Short: "blockchain commands",
		Long: `
Checks the status of the blockchain.

$ subnet-cli status blockchain [BLOCKCHAIN ID] \
--uri=http://localhost:49738 \
--check-bootstrapped

`,
		RunE: createStatusFunc,
	}

	cmd.PersistentFlags().BoolVar(&checkBootstrapped, "check-bootstrapped", false, "'true' to wait until the blockchain is bootstrapped")

	return cmd
}

var errInvalidArgs = errors.New("invalid arguments")

func createStatusFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errInvalidArgs
	}

	color.Outf("\n\n{{blue}}Setting up the configuration!{{/}}\n\n")

	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	if err != nil {
		log.Fatalf("failed to build global logger, %v", err)
	}
	_ = zap.ReplaceGlobals(logger)

	cli, err := client.New(client.Config{
		URI:            uri,
		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return err
	}

	blkChainIDs := args[0]
	blkChainID, err := ids.FromString(blkChainIDs)
	if err != nil {
		return err
	}

	opts := []internal_platformvm.OpOption{
		internal_platformvm.WithBlockchainID(blkChainID),
		internal_platformvm.WithBlockchainStatus(platformvm.Validating),
	}
	if checkBootstrapped {
		opts = append(opts, internal_platformvm.WithCheckBlockchainBootstrapped(cli.Info().Client()))
	}

	color.Outf("\n{{blue}}Checking blockchain...{{/}}\n")
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	_, err = cli.P().Checker().PollBlockchain(ctx, opts...)
	cancel()
	return err
}
