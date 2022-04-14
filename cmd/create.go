// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

// CreateCommand implements "subnet-cli create" command.
func CreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Sub-commands for creating resources",
	}
	cmd.AddCommand(
		newCreateKeyCommand(),
		newCreateSubnetCommand(),
		newCreateBlockchainCommand(),
		newCreateVMIDCommand(),
	)
	cmd.PersistentFlags().StringVar(&publicURI, "public-uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "hexadecimal-encoded private key file (either must be set between '--private-key-path' or '--ledger' but not both)")
	cmd.PersistentFlags().BoolVarP(&useLedger, "ledger", "l", false, "use ledger to sign transactions (either must be set between '--private-key-path' or '--ledger' but not both)")
	return cmd
}

func MakeCreateTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	if i.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}%s{{/}}", i.subnetIDType), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	}
	if i.blockchainID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}CREATED BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.blockchainID)})
	}
	if i.chainName != "" {
		tb.Append([]string{formatter.F("{{dark-green}}CHAIN NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.chainName)})
		tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
		tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	}
	tb.Render()
	return buf.String()
}
