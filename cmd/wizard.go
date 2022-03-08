// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"

	"github.com/ava-labs/subnet-cli/client"
	"github.com/ava-labs/subnet-cli/pkg/color"
)

// WizardCommand implements "subnet-cli wizard" command.
func WizardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wizard",
		Short: "A magical command for creating an entire subnet",
		RunE:  wizardFunc,
	}

	// "create subnet"
	cmd.PersistentFlags().StringVar(&publicURI, "public-uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")

	// "add validator"
	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")
	end := time.Now().Add(defaultValDuration)
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")

	// "create blockchain"
	cmd.PersistentFlags().StringVar(&chainName, "chain-name", "", "chain name")
	cmd.PersistentFlags().StringVar(&vmIDs, "vm-id", "", "VM ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmGenesisPath, "vm-genesis-path", "", "VM genesis file path")

	return cmd
}

func wizardFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}

	if len(nodeIDs) == 0 {
		return errors.New("no NodeIDs provided")
	}

	// Parse Args
	info.subnetID = ids.Empty
	if err := ParseNodeIDs(cli, info); err != nil {
		return err
	}
	info.stakeAmount = stakeAmount
	info.validateEnd, err = time.Parse(time.RFC3339, validateEnds)
	if err != nil {
		return err
	}
	info.validateWeight = defaultValidateWeight
	info.validateRewardFeePercent = defaultValFeePercent
	info.rewardAddr = info.key.Address()
	info.changeAddr = info.key.Address()
	info.vmID, err = ids.FromString(vmIDs)
	if err != nil {
		return err
	}
	vmGenesisBytes, err := ioutil.ReadFile(vmGenesisPath)
	if err != nil {
		return err
	}
	info.chainName = chainName
	info.vmGenesisPath = vmGenesisPath

	// Compute dry run cost/actions for approval
	info.stakeAmount = uint64(len(info.nodeIDs)) * defaultStakeAmount
	info.txFee = uint64(info.feeData.CreateSubnetTxFee) + uint64(info.feeData.TxFee)*uint64(len(info.allNodeIDs)) + uint64(info.feeData.CreateBlockchainTxFee)
	info.requiredBalance = info.stakeAmount + info.txFee
	if err := info.CheckBalance(); err != nil {
		return err
	}

	msg := CreateSpellPreTable(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to run wizard, should we continue?{{/}}\n") + msg
	}
	fmt.Fprint(formatter.ColorableStdOut, msg)

	prompt := promptui.Select{
		Label:  "\n",
		Stdout: os.Stdout,
		Items: []string{
			formatter.F("{{green}}Yes, let's create! {{bold}}{{underline}}I agree to pay the fee{{/}}{{green}}!{{/}}"),
			formatter.F("{{red}}No, stop it!{{/}}"),
		},
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return nil //nolint:nilerr
	}
	if idx == 1 {
		return nil
	}
	println()
	println()

	// Ensure all nodes are validators on the primary network
	for i, nodeID := range info.nodeIDs {
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
		if i < len(info.nodeIDs)-1 {
			info.validateEnd = info.validateEnd.Add(defaultStagger)
		}
	}
	if len(info.nodeIDs) > 0 {
		WaitValidator(cli, info.nodeIDs, info)
		println()
		println()
	}

	// Create subnet
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	subnetID, took, err := cli.P().CreateSubnet(ctx, info.key)
	cancel()
	if err != nil {
		return err
	}
	info.subnetID = subnetID
	color.Outf("{{magenta}}created subnet{{/}} %q {{light-gray}}(took %v){{/}}\n", info.subnetID, took)

	// Pause for operator to whitelist subnet on all validators (and to remind
	// that a binary by the name of [vmIDs] must be in the plugins dir)
	color.Outf("\n\n\n{{cyan}}Now, time for some config changes on your node(s).\nSet --whitelisted-subnets=%s and move the compiled VM %s to <build-dir>/plugins/%s.\nWhen you're finished, restart your node.{{/}}\n", info.subnetID, info.vmID, info.vmID)
	prompt = promptui.Select{
		Label:  "\n",
		Stdout: os.Stdout,
		Items: []string{
			formatter.F("{{green}}Yes, let's continue!{{bold}}{{underline}} I've updated --whitelisted-subnets, built my VM, and restarted my node(s)!{{/}}"),
			formatter.F("{{red}}No, stop it!{{/}}"),
		},
	}
	idx, _, err = prompt.Run()
	if err != nil {
		return nil //nolint:nilerr
	}
	if idx == 1 {
		return nil
	}
	println()
	println()

	// Add validators to subnet
	for _, nodeID := range info.allNodeIDs { // do all nodes, not parsed
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		valInfo := info.valInfos[nodeID]
		start := time.Now().Add(30 * time.Second)
		took, err := cli.P().AddSubnetValidator(
			ctx,
			info.key,
			info.subnetID,
			nodeID,
			start,
			valInfo.end,
			validateWeight,
		)
		cancel()
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}added %s to subnet %s validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, info.subnetID, took)
	}

	// Because [info.subnetID] was set to the new subnetID, [WaitValidator] will
	// lookup status for subnetID
	WaitValidator(cli, info.allNodeIDs, info)
	println()
	println()

	// Add blockchain to subnet
	ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
	blockchainID, took, err := cli.P().CreateBlockchain(
		ctx,
		info.key,
		info.subnetID,
		info.chainName,
		info.vmID,
		vmGenesisBytes,
	)
	cancel()
	if err != nil {
		return err
	}
	info.blockchainID = blockchainID
	color.Outf("{{magenta}}created blockchain{{/}} %q {{light-gray}}(took %v){{/}}\n\n", info.blockchainID, took)

	// Print out summary of actions (subnetID, chainID, validator periods)
	info.requiredBalance = 0
	info.stakeAmount = 0
	info.txFee = 0
	info.balance, err = cli.P().Balance(context.Background(), info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, CreateSpellPostTable(info))
	return nil
}

func CreateSpellPreTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	if len(i.nodeIDs) > 0 {
		tb.Append([]string{formatter.F("{{magenta}}NEW PRIMARY NETWORK VALIDATORS{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.nodeIDs)})
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateEnd.Format(time.RFC3339))})
		stakeAmount := float64(i.stakeAmount) / float64(units.Avax)
		stakeAmounts := humanize.FormatFloat("#,###.###", stakeAmount)
		tb.Append([]string{formatter.F("{{magenta}}STAKE AMOUNT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}} $AVAX", stakeAmounts)})
		validateRewardFeePercent := humanize.FormatFloat("#,###.###", float64(i.validateRewardFeePercent))
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE REWARD FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} %%", validateRewardFeePercent)})
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}REWARD ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.rewardAddr)})
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}CHANGE ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.changeAddr)})
	}

	tb.Append([]string{formatter.F("{{orange}}NEW SUBNET VALIDATORS{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.allNodeIDs)})
	tb.Append([]string{formatter.F("{{magenta}}SUBNET VALIDATION WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(i.validateWeight)))})

	tb.Append([]string{formatter.F("{{dark-green}}CHAIN NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.chainName)})
	tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
	tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	tb.Render()
	return buf.String()
}

func CreateSpellPostTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	if len(i.nodeIDs) > 0 {
		tb.Append([]string{formatter.F("{{magenta}}PRIMARY NETWORK VALIDATORS{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.nodeIDs)})
	}

	tb.Append([]string{formatter.F("{{orange}}SUBNET VALIDATORS{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.allNodeIDs)})
	tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	tb.Append([]string{formatter.F("{{blue}}BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.blockchainID)})

	tb.Append([]string{formatter.F("{{dark-green}}CHAIN NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.chainName)})
	tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
	tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	tb.Render()
	return buf.String()
}
