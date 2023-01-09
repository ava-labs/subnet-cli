// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

func newRemoveSubnetValidatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subnet-validator",
		Short: "Removes a subnet to the validator",
		Long: `
Removes a subnet validator.

$ subnet-cli add subnet-validator \
--private-key-path=.insecure.ewoq.key \
--public-uri=http://localhost:52250 \
--subnet-id="24tZhrm8j8GCJRE9PomW8FaeqbgGS4UAQjJnqqn8pq5NwYSYV1" \
--node-ids="NodeID-4B4rc5vdD1758JSBYL1xyvE5NHGzz6xzH"
`,
		RunE: removeSubnetValidatorFunc,
	}

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringSliceVar(&nodeIDs, "node-ids", nil, "a list of node IDs (must be formatted in ids.ID)")

	return cmd
}

func removeSubnetValidatorFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}
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
	info.txFee *= uint64(len(info.nodeIDs))
	info.requiredBalance = info.txFee
	if err := info.CheckBalance(); err != nil {
		return err
	}
	msg := CreateRemoveValidator(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to remove subnet validator, should we continue?{{/}}\n") + msg
	}
	fmt.Fprint(formatter.ColorableStdOut, msg)

	if enablePrompt {
		prompt := promptui.Select{
			Label:  "\n",
			Stdout: os.Stdout,
			Items: []string{
				formatter.F("{{green}}Yes, let's remove! {{bold}}{{underline}}I agree to pay the fee{{/}}{{green}}!{{/}}"),
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
		took, err := cli.P().RemoveSubnetValidator(
			ctx,
			info.key,
			info.subnetID,
			nodeID,
		)
		cancel()
		if err != nil {
			return err
		}
		color.Outf("{{magenta}}removed %s from subnet %s validator set{{/}} {{light-gray}}(took %v){{/}}\n\n", nodeID, info.subnetID, took)
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
