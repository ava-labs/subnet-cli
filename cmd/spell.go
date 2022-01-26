// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

	"github.com/ava-labs/avalanchego/ids"
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
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}

	originalNodeIDs := 2 // TODO: real number
	info.subnetID = ids.Empty
	if err := ParseNodeIDs(cli, info); err != nil {
		return err
	}

	// Compute dry run cost/actions for approval
	defaultStakeAmount := 1 * units.Avax
	requiredAvax := uint64(len(info.nodeIDs))*defaultStakeAmount + uint64(info.feeData.CreateSubnetTxFee) + uint64(info.feeData.TxFee)*uint64(originalNodeIDs) + uint64(info.feeData.CreateBlockchainTxFee)

	if err := info.CheckBalance(); err != nil {
		return err
	}

	msg := CreateAddTable(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to add validator, should we continue?{{/}}\n") + msg
	}
	fmt.Fprint(formatter.ColorableStdOut, msg)

	if enablePrompt {
		prompt := promptui.Select{
			Label:  "\n",
			Stdout: os.Stdout,
			Items: []string{
				formatter.F("{{red}}No, stop it!{{/}}"),
				formatter.F("{{green}}Yes, let's create! {{bold}}{{underline}}I agree to pay the fee{{/}}{{green}}!{{/}}"),
			},
		}
		idx, _, err := prompt.Run()
		if err != nil {
			panic(err)
		}
		if idx == 0 {
			return nil
		}
	}

	// Ensure all nodes are validators on the primary network
	for _, nodeID := range info.nodeIDs {
		// add validator
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		took, err := cli.P().AddValidator(
			ctx,
			info.key,
			nodeID,
			info.validateStart,
			info.validateEnd,
			client.WithStakeAmount(info.stakeAmount),
			client.WithRewardShares(info.validateRewardFeePercent*10000),
			client.WithRewardAddress(info.rewardAddr),
			client.WithChangeAddress(info.changeAddr),
		)
		cancel()
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}added %s to primary network validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, took)
	}

	// Create subnet
	ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
	subnetID, took, err := cli.P().CreateSubnet(ctx, info.key, client.WithDryMode(false))
	cancel()
	if err != nil {
		return err
	}

	// Pause for operator to whitelist subnet on all validators (and to remind
	// that a binary by the name of [vmIDs] must be in the plugins dir)
	if enablePrompt {
		prompt := promptui.Select{
			Label:  "\n",
			Stdout: os.Stdout,
			Items: []string{
				formatter.F("{{red}}No, stop it!{{/}}"),
				formatter.F("{{green}}Yes, let's create! {{bold}}{{underline}}I agree to pay the fee{{/}}{{green}}!{{/}}"),
			},
		}
		idx, _, err := prompt.Run()
		if err != nil {
			panic(err)
		}
		if idx == 0 {
			return nil
		}
	}

	// Add validators to subnet
	for _, nodeID := range info.nodeIDs { // do all nodes, not parsed
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		took, err := cli.P().AddSubnetValidator(
			ctx,
			info.key,
			info.subnetID,
			nodeID,
			info.validateStart,
			info.validateEnd,
			validateWeight,
		)
		cancel()
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}added %s to subnet %s validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, info.subnetID, took)
	}

	// Add blockchain to subnet
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	blockchainID, took, err := cli.P().CreateBlockchain(
		ctx,
		info.key,
		info.subnetID,
		info.chainName,
		info.vmID,
		vmGenesisBytes,
		client.WithPoll(false),
	)
	cancel()

	// Print out summary of actions (subnetID, chainID, validator periods)
	info.txFee = 0
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, MakeCreateTable(info))
	return nil
}
