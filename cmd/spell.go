// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

// SpellCommand implements "subnet-cli add" command.
func SpellCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spell",
		Short: "A sub-command for creating resources",
		RunE:  spellFunc,
	}

	// "create subnet"
	cmd.PersistentFlags().StringVar(&publicURI, "public-uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")

	// "add validator"
	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")
	start := time.Now().Add(30 * time.Second)
	end := start.Add(2 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint64Var(&validateWeight, "validate-weight", 1000, "validate weight")

	// "create blockchain"
	cmd.PersistentFlags().StringVar(&chainName, "chain-name", "", "chain name")
	cmd.PersistentFlags().StringVar(&vmIDs, "vm-id", "", "VM ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmGenesisPath, "vm-genesis-path", "", "VM genesis file path")

	return cmd
}

func spellFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}

	// Ensure validators are network validators

	if err := createSubnet(cli, info); err != nil {
		return err
	}

	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	if err := addValidator(cli, info); err != nil {
		return err
	}
	if err := createBlockchain(cli, info); err != nil {
		return err
	}

	return nil
}
