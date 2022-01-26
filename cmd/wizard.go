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
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"

	"github.com/ava-labs/subnet-cli/internal/client"
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
	start := time.Now().Add(30 * time.Second)
	end := start.Add(30 * 24 * time.Hour)
	// TODO: stagger end times for validators by 2 hours
	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
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
	info.validateStart, err = time.Parse(time.RFC3339, validateStarts)
	if err != nil {
		return err
	}
	info.validateEnd, err = time.Parse(time.RFC3339, validateEnds)
	if err != nil {
		return err
	}
	info.validateWeight = defaultValidateWeight
	info.validateRewardFeePercent = defaultValFeePercent
	info.rewardAddr = info.key.Key().PublicKey().Address()
	info.changeAddr = info.key.Key().PublicKey().Address()
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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	subnetID, took, err := cli.P().CreateSubnet(ctx, info.key)
	cancel()
	if err != nil {
		return err
	}
	info.subnetID = subnetID

	// Pause for operator to whitelist subnet on all validators (and to remind
	// that a binary by the name of [vmIDs] must be in the plugins dir)
	color.Outf("{{cyan}}Now that the subnet is created, add %s to --whitelisted-subnets and %s to <build-dir/plugins. Then, restart your node.{{/}}\n", info.subnetID, info.vmID)
	prompt := promptui.Select{
		Label:  "\n",
		Stdout: os.Stdout,
		Items: []string{
			formatter.F("{{green}}Yes, let's continue!{{bold}}{{underline}}I've updated --whitelisted-subnets, built my VM, and restarted my node!{{/}}"),
		},
	}
	if _, _, err := prompt.Run(); err != nil {
		panic(err)
	}

	// Add validators to subnet
	for _, nodeID := range info.nodeIDs { // do all nodes, not parsed
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		valInfo := info.valInfos[nodeID]
		took, err := cli.P().AddSubnetValidator(
			ctx,
			info.key,
			info.subnetID,
			nodeID,
			valInfo.start,
			valInfo.end,
			validateWeight,
		)
		cancel()
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}added %s to subnet %s validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, info.subnetID, took)
	}

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
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, CreateSpellPostTable(info))
	return nil
}

func CreateSpellPreTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	tb.Append([]string{formatter.F("{{orange}}NODE IDs{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.allNodeIDs)})

	if len(i.nodeIDs) > 0 {
		tb.Append([]string{formatter.F("{{magenta}}NEW PRIMARY NETWORK VALIDATORS{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.nodeIDs)})
		tb.Append([]string{formatter.F("{{magenta}}STAKE AMOUNT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(i.stakeAmount)))})
		validateRewardFeePercent := humanize.FormatFloat("#,###.###", float64(i.validateRewardFeePercent))
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE REWARD FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} %%", validateRewardFeePercent)})
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}REWARD ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.rewardAddr)})
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}CHANGE ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.changeAddr)})
	}

	tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateStart.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateEnd.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(i.validateWeight)))})

	tb.Append([]string{formatter.F("{{dark-green}}CHAIN NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.chainName)})
	tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
	tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	tb.Render()
	return buf.String()
}

func CreateSpellPostTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	tb.Append([]string{formatter.F("{{orange}}NODE IDs{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.allNodeIDs)})

	tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	tb.Append([]string{formatter.F("{{blue}}BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.blockchainID)})

	tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateStart.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateEnd.Format(time.RFC3339))})

	tb.Append([]string{formatter.F("{{dark-green}}CHAIN NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.chainName)})
	tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
	tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	tb.Render()
	return buf.String()
}
