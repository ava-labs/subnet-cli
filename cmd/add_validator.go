// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/subnet-cli/client"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

const (
	defaultStakeAmount   = 1 * units.Avax
	defaultValFeePercent = 2
	defaultStagger       = 2 * time.Hour
	defaultValDuration   = 300 * 24 * time.Hour
)

func newAddValidatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Adds a node as a validator",
		Long: `
Adds a node as a validator.

$ subnet-cli add validator \
--private-key-path=.insecure.ewoq.key \
--public-uri=http://localhost:52250 \
--node-ids="NodeID-4B4rc5vdD1758JSBYL1xyvE5NHGzz6xzH" \
--stake-amount=2000000000000 \
--validate-reward-fee-percent=2

`,
		RunE: createValidatorFunc,
	}

	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")
	cmd.PersistentFlags().Uint64Var(&stakeAmount, "stake-amount", defaultStakeAmount, "stake amount denominated in nano AVAX (minimum amount that a validator must stake is 2,000 AVAX)")

	end := time.Now().Add(300 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint32Var(&validateRewardFeePercent, "validate-reward-fee-percent", defaultValFeePercent, "percentage of fee that the validator will take rewards from its delegators")
	cmd.PersistentFlags().StringVar(&rewardAddrs, "reward-address", "", "node address to send rewards to (default to key owner)")
	cmd.PersistentFlags().StringVar(&changeAddrs, "change-address", "", "node address to send changes to (default to key owner)")

	return cmd
}

var errInvalidValidateRewardFeePercent = errors.New("invalid validate reward fee percent")

func createValidatorFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}
	info.stakeAmount = stakeAmount

	info.subnetID = ids.Empty
	if err := ParseNodeIDs(cli, info); err != nil {
		return err
	}
	if len(info.nodeIDs) == 0 {
		color.Outf("{{magenta}}no primary network validators to add{{/}}\n")
		return nil
	}
	info.validateEnd, err = time.Parse(time.RFC3339, validateEnds)
	if err != nil {
		return err
	}

	info.validateWeight = 0
	info.validateRewardFeePercent = validateRewardFeePercent
	if info.validateRewardFeePercent < 2 {
		return errInvalidValidateRewardFeePercent
	}

	if rewardAddrs != "" {
		info.rewardAddr, err = ids.ShortFromPrefixedString(rewardAddrs, constants.NodeIDPrefix)
		if err != nil {
			return err
		}
	} else {
		info.rewardAddr = info.key.Key().PublicKey().Address()
	}
	if changeAddrs != "" {
		info.changeAddr, err = ids.ShortFromPrefixedString(changeAddrs, constants.NodeIDPrefix)
		if err != nil {
			return err
		}
	} else {
		info.changeAddr = info.key.Key().PublicKey().Address()
	}
	info.requiredBalance = info.stakeAmount * uint64(len(info.nodeIDs))
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

	println()
	println()
	println()
	for _, nodeID := range info.nodeIDs {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		info.validateStart = time.Now().Add(30 * time.Second)
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
		info.validateEnd = info.validateEnd.Add(defaultStagger)
	}
	info.requiredBalance = 0
	info.stakeAmount = 0
	info.txFee = 0
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, CreateAddTable(info))
	return nil
}
