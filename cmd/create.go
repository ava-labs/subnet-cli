// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"bytes"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
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
	)
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", ".subnet-cli.pk", "private key file path")
	return cmd
}

func MakeCreateTable(i *Info) string {
	// P-Chain balance is denominated by units.Avax or 10^9 nano-Avax
	curPChainDenominatedP := float64(i.balance) / float64(units.Avax)
	curPChainDenominatedBalanceP := humanize.FormatFloat("#,###.#######", curPChainDenominatedP)

	txFee := float64(i.txFee) / float64(units.Avax)
	txFees := humanize.FormatFloat("#,###.###", txFee)

	buf := bytes.NewBuffer(nil)
	tb := tablewriter.NewWriter(buf)

	tb.SetAutoWrapText(false)
	tb.SetColWidth(1500)
	tb.SetCenterSeparator("*")

	tb.SetRowLine(true)
	tb.SetAlignment(tablewriter.ALIGN_LEFT)

	tb.Append([]string{formatter.F("{{cyan}}P-CHAIN ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.key.P())})
	tb.Append([]string{formatter.F("{{coral}}{{bold}}P-CHAIN BALANCE{{/}} "), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", curPChainDenominatedBalanceP)})
	tb.Append([]string{formatter.F("{{red}}{{bold}}TX FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", txFees)})
	tb.Append([]string{formatter.F("{{orange}}URI{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", uri)})
	tb.Append([]string{formatter.F("{{orange}}NETWORK NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.networkName)})

	if i.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}%s{{/}}", i.subnetIDType), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.subnetID)})
	}

	if i.blockchainID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}CREATED BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.blockchainID)})
	}
	if i.vmName != "" {
		tb.Append([]string{formatter.F("{{dark-green}}VM NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmName)})
		tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmID)})
		tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.vmGenesisPath)})
	}

	tb.Render()
	return buf.String()
}
