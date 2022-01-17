// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

func newAddValidatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Adds a subnet to the validator",
		Long: `
Adds a subnet to the validator.

$ subnet-cli add validator \
--private-key-path=.insecure.ewoq.key \
--public-uri=http://localhost:52250 \
--subnet-id="24tZhrm8j8GCJRE9PomW8FaeqbgGS4UAQjJnqqn8pq5NwYSYV1" \
--node-id="NodeID-4B4rc5vdD1758JSBYL1xyvE5NHGzz6xzH" \
--validate-weight=1000

`,
		RunE: createValidatorFunc,
	}

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&nodeIDs, "node-id", "", "node ID (must be formatted in ids.ID)")

	start := time.Now().Add(30 * time.Second)
	end := start.Add(2 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint64Var(&validateWeight, "validate-weight", 1000, "validate weight")
	return cmd
}

func createValidatorFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}
	info.txFee = uint64(info.feeData.TxFee)
	info.nodeID, err = ids.ShortFromPrefixedString(nodeIDs, constants.NodeIDPrefix)
	if err != nil {
		return err
	}
	info.subnetID, err = ids.FromString(subnetIDs)
	if err != nil {
		return err
	}
	info.validateStart, err = time.Parse(time.RFC3339, validateStarts)
	if err != nil {
		return err
	}
	info.validateEnd, err = time.Parse(time.RFC3339, validateEnds)
	if err != nil {
		return err
	}
	info.validateWeight = validateWeight
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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	took, err := cli.P().AddSubnetValidator(
		ctx,
		info.key,
		info.subnetID,
		info.nodeID,
		info.validateStart,
		info.validateEnd,
		validateWeight,
	)
	cancel()
	if err != nil {
		return err
	}
	color.Outf("{{magenta}}added subnet to validator{{/}} %q {{light-gray}}(took %v){{/}}\n\n", info.subnetID, took)

	info.txFee = 0
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, CreateAddTable(info))
	return nil
}
