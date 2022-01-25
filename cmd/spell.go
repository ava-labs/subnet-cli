// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

	"github.com/ava-labs/avalanchego/utils/units"
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

	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")

	// "add validator"
	cmd.PersistentFlags().Uint64Var(&stakeAmount, "stake-amount", 1*units.Avax, "stake amount denominated in nano AVAX (minimum amount that a validator must stake is 2,000 AVAX)")
	start := time.Now().Add(30 * time.Second)
	end := start.Add(60 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint32Var(&validateRewardFeePercent, "validate-reward-fee-percent", 2, "percentage of fee that the validator will take rewards from its delegators")
	cmd.PersistentFlags().StringVar(&rewardAddrs, "reward-address", "", "node address to send rewards to (default to key owner)")
	cmd.PersistentFlags().StringVar(&changeAddrs, "change-address", "", "node address to send changes to (default to key owner)")

	// "add subnet-validator"
	subnetStart := start.Add(time.Minute)
	subnetEnd := subnetStart.Add(50 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&subnetValidateStarts, "subnet-validate-start", subnetStart.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&subnetValidateEnds, "subnet-validate-end", subnetEnd.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint64Var(&subnetValidateWeight, "subnet-validate-weight", 1000, "validate weight")

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

	if err := addSubnetValidator(cli, info); err != nil {
		return err
	}
	if err := createBlockchain(cli, info); err != nil {
		return err
	}

	return nil
}
