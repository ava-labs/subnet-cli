// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/subnet-cli/client"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

func newCreateBlockchainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blockchain [options]",
		Short: "Creates a blockchain",
		Long: `
Creates a blockchain.

$ subnet-cli create blockchain \
--private-key-path=.insecure.ewoq.key \
--subnet-id="24tZhrm8j8GCJRE9PomW8FaeqbgGS4UAQjJnqqn8pq5NwYSYV1" \
--chain-name=my-custom-chain \
--vm-id=tGas3T58KzdjLHhBDMnH2TvrddhqTji5iZAMZ3RXs2NLpSnhH \
--vm-genesis-path=.my-custom-vm.genesis

`,
		RunE: createBlockchainFunc,
	}

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&chainName, "chain-name", "", "chain name")
	cmd.PersistentFlags().StringVar(&vmIDs, "vm-id", "", "VM ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmGenesisPath, "vm-genesis-path", "", "VM genesis file path")

	return cmd
}

func createBlockchainFunc(cmd *cobra.Command, args []string) error {
	cli, info, err := InitClient(publicURI, true)
	if err != nil {
		return err
	}
	info.subnetIDType = "SUBNET ID"
	info.subnetID, err = ids.FromString(subnetIDs)
	if err != nil {
		return err
	}
	info.vmID, err = ids.FromString(vmIDs)
	if err != nil {
		return err
	}
	vmGenesisBytes, err := ioutil.ReadFile(vmGenesisPath)
	if err != nil {
		return err
	}
	info.txFee = uint64(info.feeData.CreateBlockchainTxFee)
	info.requiredBalance = info.txFee
	if err := info.CheckBalance(); err != nil {
		return err
	}
	info.chainName = chainName
	info.vmGenesisPath = vmGenesisPath

	msg := MakeCreateTable(info)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to create blockchain resources, should we continue?{{/}}\n") + msg
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

	info.requiredBalance = 0
	info.stakeAmount = 0
	info.txFee = 0
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return err
	}
	fmt.Fprint(formatter.ColorableStdOut, MakeCreateTable(info))
	return nil
}
