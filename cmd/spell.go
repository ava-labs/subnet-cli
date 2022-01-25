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
	// TODO: stagger end times for validators by 2 hours
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
	// cli, info, err := InitClient(publicURI, true)
	// if err != nil {
	// 	return err
	// }

	// Compute dry run cost/actions for approval

	// Ensure all nodes are validators on the primary network

	// Create subnet

	// Pause for operator to whitelist subnet on all validators (and to remind
	// that a binary by the name of [vmIDs] must be in the plugins dir)

	// Add validators to subnet

	// Add blockchain to subnet

	// Print out summary of actions (subnetID, chainID, validator periods)

	return nil
}
