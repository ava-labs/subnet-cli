// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

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
	)

	cmd.PersistentFlags().StringVar(&uri, "uri", "https://api.avax-test.network", "URI for avalanche network endpoints")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")
	return cmd
}

func CreateAddTable(i *Info) string {
	buf, tb := BaseTableSetup(i)
	tb.Append([]string{formatter.F("{{orange}}NODE ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.nodeID)})
	tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateStart.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.validateEnd.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(i.validateWeight)))})
	tb.Render()
	return buf.String()
}
