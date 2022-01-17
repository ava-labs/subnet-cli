// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

func newCreateSubnetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subnet",
		Short: "Creates a subnet",
		Long: `
Creates a subnet based on the configuration.

$ subnet-cli create subnet \
--private-key-path=.insecure.ewoq.key \
--uri=http://localhost:52250

`,
		RunE: createSubnetFunc,
	}

	return cmd
}

func createSubnetFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	sid, _, err := cli.P().CreateSubnet(ctx, info.key, client.WithDryMode(true))
	cancel()
	if err != nil {
		return err
	}
	info.txFee = uint64(info.feeData.CreateSubnetTxFee)
	info.subnetIDType = "EXPECTED SUBNET ID"
	info.subnetID = sid
	if info.CheckBalance(); err != nil {
		return err
	}

	msg := MakeCreateTable(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to create subnet resources, should we continue?{{/}}\n") + msg
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
	ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
	subnetID, took, err := cli.P().CreateSubnet(ctx, info.key, client.WithDryMode(false))
	cancel()
	if err != nil {
		return err
	}
	info.subnetIDType = "CREATED SUBNET ID"
	info.subnetID = subnetID

	color.Outf("{{magenta}}created subnet{{/}} %q {{light-gray}}(took %v){{/}}\n", info.subnetID, took)
	color.Outf("({{orange}}subnet must be whitelisted beforehand via{{/}} {{cyan}}{{bold}}--whitelisted-subnets{{/}} {{orange}}flag!{{/}})\n\n")

	info.txFee = 0
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, MakeCreateTable(info))
	return nil
}
