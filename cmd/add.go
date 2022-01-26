// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/dustin/go-humanize"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

// AddCommand implements "subnet-cli add" command.
func AddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Sub-commands for creating resources",
	}
	cmd.AddCommand(
		newAddValidatorCommand(),
		newAddSubnetValidatorCommand(),
	)
	cmd.PersistentFlags().StringVar(&publicURI, "public-uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")
	return cmd
}

func CreateAddTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	tb.Append([]string{formatter.F("{{orange}}NODE IDs{{/}}"), formatter.F("{{light-gray}}{{bold}}%v{{/}}", i.nodeIDs)})
	if i.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	}
	if !i.validateStart.IsZero() {
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateStart.Format(time.RFC3339))})
	}
	if !i.validateEnd.IsZero() {
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateEnd.Format(time.RFC3339))})
	}
	if i.validateWeight > 0 {
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(i.validateWeight)))})
	}
	if i.validateRewardFeePercent > 0 {
		validateRewardFeePercent := humanize.FormatFloat("#,###.###", float64(i.validateRewardFeePercent))
		tb.Append([]string{formatter.F("{{magenta}}VALIDATE REWARD FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} %%", validateRewardFeePercent)})
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
