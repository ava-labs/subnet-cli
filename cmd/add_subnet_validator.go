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
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

const (
	defaultValidateWeight = 1000
)

func newAddSubnetValidatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subnet-validator",
		Short: "Adds a subnet to the validator",
		Long: `
Adds a subnet to the validator.

$ subnet-cli add subnet-validator \
--private-key-path=/tmp/test.key \
--public-uri=http://aops-custom-202204-px4TcM-nlb-f82ea9a371a1b3d4.elb.us-west-2.amazonaws.com:9650 \
--subnet-id="ukpctkqaeqAR8RMgLC9fLALoRCY4ovtgaCLoxdUU89qw8zpsf" \
--node-ids="NodeID-PqKwnUuz2b1aAJRA8LzS3RgF1T6Y1UovS,NodeID-BAAufbVWn6gyNERkYBGCaqYNMT3Tv4xjE,NodeID-NSHcp5W19kbJTjxMWgYbfNHNY1oJCL9Xi,NodeID-KQu7yZRUmRnaCrzoJnvQqGpzeYTJ4GC1L" \
--validate-weight=1000

`,
		RunE: createSubnetValidatorFunc,
	}

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")
	cmd.PersistentFlags().Uint64Var(&validateWeight, "validate-weight", defaultValidateWeight, "validate weight")

	return cmd
}

var errZeroValidateWeight = errors.New("zero validate weight")

func createSubnetValidatorFunc(cmd *cobra.Command, args []string) error {
	baseWallet, cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}
	fmt.Println("subnetIDs:", subnetIDs)
	info.subnetID, err = ids.FromString(subnetIDs)
	if err != nil {
		return err
	}
	info.txFee = uint64(info.feeData.TxFee)
	if err := ParseNodeIDs(cli, info); err != nil {
		return err
	}
	if len(info.nodeIDs) == 0 {
		color.Outf("{{magenta}}no subnet validators to add{{/}}\n")
		return nil
	}

	info.validateWeight = validateWeight
	info.validateRewardFeePercent = 0
	if info.validateWeight == 0 {
		return errZeroValidateWeight
	}

	info.rewardAddr = ids.ShortEmpty
	info.changeAddr = ids.ShortEmpty

	info.txFee *= uint64(len(info.nodeIDs))
	info.requiredBalance = info.txFee
	if err := info.CheckBalance(); err != nil {
		return err
	}
	msg := CreateAddTable(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to add subnet validator, should we continue?{{/}}\n") + msg
	}
	fmt.Fprint(formatter.ColorableStdOut, msg)

	if enablePrompt {
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
	}

	println()
	println()
	println()
	for _, nodeID := range info.nodeIDs {
		// valInfo is not populated because [ParseNodeIDs] called on info.subnetID
		//
		// TODO: cleanup
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		_, end, err := cli.P().GetValidator(ctx, ids.Empty, nodeID)
		cancel()
		if err != nil {
			return err
		}
		info.validateStart = time.Now().Add(30 * time.Second)
		info.validateEnd = end

		var took time.Duration
		if baseWallet != nil {
			// TODO: "wallet/chain/p.GetTx" on the subnet tx ID does not work...
			// e.g, failed to fetch subnet "ukpctkqaeqAR8RMgLC9fLALoRCY4ovtgaCLoxdUU89qw8zpsf": not found
			// this is because "wallet/chain/p.backend.AcceptTx" was not called to update "b.txs"
			// which is used for "GetTx"...
			// should we fallback to DB call if tx is not found in cache?
			start := time.Now()
			_, err = baseWallet.P().IssueAddSubnetValidatorTx(
				&platformvm.SubnetValidator{
					Validator: platformvm.Validator{
						NodeID: nodeID,
						Start:  uint64(info.validateStart.Unix()),
						End:    uint64(info.validateEnd.Unix()),
						Wght:   info.stakeAmount,
					},
					Subnet: info.subnetID,
				},
				common.WithPollFrequency(5*time.Second),
			)
			took = time.Since(start)
		} else {
			ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
			took, err = cli.P().AddSubnetValidator(
				ctx,
				info.key,
				info.subnetID,
				nodeID,
				info.validateStart,
				info.validateEnd,
				validateWeight,
			)
			cancel()
		}
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}added %s to subnet %s validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, info.subnetID, took)
	}
	WaitValidator(cli, info.nodeIDs, info)
	info.requiredBalance = 0
	info.stakeAmount = 0
	info.txFee = 0
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	info.balance, err = cli.P().Balance(ctx, info.key)
	cancel()
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, CreateAddTable(info))
	return nil
}
