// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package implements "status" sub-commands.
package cmd

import (
	"context"

	"github.com/ava-labs/avalanchego/ids"
	pstatus "github.com/ava-labs/avalanchego/vms/platformvm/status"
	internal_platformvm "github.com/ava-labs/subnet-cli/internal/platformvm"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/spf13/cobra"
)

func newStatusBlockchainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blockchain [BLOCKCHAIN ID]",
		Short: "blockchain commands",
		Long: `
Checks the status of the blockchain.

$ subnet-cli status blockchain \
--blockchain-id=[BLOCKCHAIN ID] \
--private-uri=http://localhost:49738 \
--check-bootstrapped

`,
		RunE: createStatusFunc,
	}

	cmd.PersistentFlags().StringVar(&blockchainID, "blockchain-id", "", "blockchain to check the status of")
	cmd.PersistentFlags().BoolVar(&checkBootstrapped, "check-bootstrapped", false, "'true' to wait until the blockchain is bootstrapped")
	return cmd
}

func createStatusFunc(cmd *cobra.Command, args []string) error {
	_, cli, _, err := InitClient(privateURI, false)
	if err != nil {
		return err
	}

	blkChainID, err := ids.FromString(blockchainID)
	if err != nil {
		return err
	}

	opts := []internal_platformvm.OpOption{
		internal_platformvm.WithBlockchainID(blkChainID),
		internal_platformvm.WithBlockchainStatus(pstatus.Validating),
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
