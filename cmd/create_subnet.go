// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/subnet-cli/client"
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
--public-uri=http://localhost:52250

`,
		RunE: createSubnetFunc,
	}

	return cmd
}

func createSubnetFunc(cmd *cobra.Command, args []string) error {
	baseWallet, cli, info, err := InitClient(publicURI, true)
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
	if err := info.CheckBalance(); err != nil {
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
	var subnetID ids.ID
	var took time.Duration
	if baseWallet != nil {
		start := time.Now()
		subnetID, err = baseWallet.P().IssueCreateSubnetTx(
			&secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     info.key.Addresses(),
			},
			common.WithPollFrequency(5*time.Second),
		)
		took = time.Since(start)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
		subnetID, took, err = cli.P().CreateSubnet(ctx, info.key)
		cancel()
	}
	if err != nil {
		return err
	}
	info.subnetIDType = "CREATED SUBNET ID"
	info.subnetID = subnetID

	color.Outf("{{magenta}}created subnet{{/}} %q {{light-gray}}(took %v){{/}}\n", info.subnetID, took)
	color.Outf("({{orange}}subnet must be whitelisted beforehand via{{/}} {{cyan}}{{bold}}--whitelisted-subnets{{/}} {{orange}}flag!{{/}})\n\n")

	info.requiredBalance = 0
	info.stakeAmount = 0
	info.txFee = 0
	ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
	info.balance, err = cli.P().Balance(ctx, info.key)
	cancel()
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, MakeCreateTable(info))
	return nil
}
