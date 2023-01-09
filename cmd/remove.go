// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

// RemoveCommand implements "subnet-cli add" command.
func RemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Sub-commands for removing resources",
	}
	cmd.AddCommand(
		newRemoveSubnetValidatorCommand(),
	)
	cmd.PersistentFlags().StringVar(&publicURI, "public-uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")
	cmd.PersistentFlags().BoolVarP(&useLedger, "ledger", "l", false, "use ledger to sign transactions")
	return cmd
}

func CreateRemoveValidator(i *Info) string {
	buf, tb := BaseTableSetup(i)
	tb.Append([]string{formatter.F("{{orange}}NODE IDs{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.nodeIDs)})
	if i.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	}
	if i.rewardAddr != ids.ShortEmpty {
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}REWARD ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.rewardAddr)})
	}
	if i.changeAddr != ids.ShortEmpty {
		tb.Append([]string{formatter.F("{{cyan}}{{bold}}CHANGE ADDRESS{{/}}"), formatter.F("{{light-gray}}%s{{/}}", i.changeAddr)})
	}
	tb.Render()
	return buf.String()
}
