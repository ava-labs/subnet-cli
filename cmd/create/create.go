// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package implements "create" sub-commands.
package create

import (
	"bytes"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

func init() {
	cobra.EnablePrefixMatching = true
}

var (
	enablePrompt bool
	logLevel     string

	privKeyPath string

	uri string

	pollInterval   time.Duration
	requestTimeout time.Duration
)

// NewCommand implements "subnet-cli create" command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Sub-commands for creating resources",
	}
	cmd.AddCommand(
		newCreateKeyCommand(),
		newCreateSubnetCommand(),
		newCreateBlockchainCommand(),
	)
	return cmd
}

type status struct {
	curPChainBalance      uint64
	txFee                 uint64
	subnetTxFee           uint64
	createBlockchainTxFee uint64
	afterPChainBalance    uint64

	key key.Key

	uri         string
	networkName string

	pollInterval   time.Duration
	requestTimeout time.Duration

	subnetIDType string
	subnetID     ids.ID

	blkChainID    ids.ID
	vmName        string
	vmID          ids.ID
	vmGenesisPath string
}

func (m status) Table(before bool) string {
	// P-Chain balance is denominated by units.Avax or 10^9 nano-Avax
	curPChainDenominatedP := float64(m.curPChainBalance) / float64(units.Avax)
	curPChainDenominatedBalanceP := humanize.FormatFloat("#,###.#######", curPChainDenominatedP)

	subnetTxFee := float64(m.subnetTxFee) / float64(units.Avax)
	subnetTxFees := humanize.FormatFloat("#,###.###", subnetTxFee)

	txFee := float64(m.txFee) / float64(units.Avax)
	txFees := humanize.FormatFloat("#,###.###", txFee)

	createBlockchainTxFee := float64(m.createBlockchainTxFee) / float64(units.Avax)
	createBlockchainTxFees := humanize.FormatFloat("#,###.###", createBlockchainTxFee)

	afterPChainDenominatedP := float64(m.afterPChainBalance) / float64(units.Avax)
	afterPChainDenominatedBalanceP := humanize.FormatFloat("#,###.#######", afterPChainDenominatedP)

	buf := bytes.NewBuffer(nil)
	tb := tablewriter.NewWriter(buf)

	tb.SetAutoWrapText(false)
	tb.SetColWidth(1500)
	tb.SetCenterSeparator("*")

	tb.SetRowLine(true)
	tb.SetAlignment(tablewriter.ALIGN_LEFT)

	tb.Append([]string{formatter.F("{{coral}}{{bold}}CURRENT P-CHAIN BALANCE{{/}} "), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", curPChainDenominatedBalanceP)})
	tb.Append([]string{formatter.F("{{red}}{{bold}}TX FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", txFees)})
	tb.Append([]string{formatter.F("{{red}}{{bold}}CREATE SUBNET TX FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", subnetTxFees)})
	tb.Append([]string{formatter.F("{{red}}{{bold}}CREATE BLOCKCHAIN TX FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", createBlockchainTxFees)})
	if before {
		tb.Append([]string{formatter.F("{{coral}}{{bold}}ESTIMATED P-CHAIN BALANCE{{/}} "), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", afterPChainDenominatedBalanceP)})
	}

	tb.Append([]string{formatter.F("{{cyan}}X-CHAIN ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.key.X())})
	tb.Append([]string{formatter.F("{{cyan}}P-CHAIN ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.key.P())})
	tb.Append([]string{formatter.F("{{cyan}}C-CHAIN ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.key.C())})
	tb.Append([]string{formatter.F("{{cyan}}ETH ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.key.Eth().String())})
	tb.Append([]string{formatter.F("{{cyan}}SHORT ADDRESS{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.key.Short().String())})

	tb.Append([]string{formatter.F("{{orange}}URI{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.uri)})
	tb.Append([]string{formatter.F("{{orange}}NETWORK NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.networkName)})
	tb.Append([]string{formatter.F("{{orange}}POLL INTERVAL{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.pollInterval)})
	tb.Append([]string{formatter.F("{{orange}}REQUEST TIMEOUT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.requestTimeout)})

	if m.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}%s{{/}}", m.subnetIDType), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.subnetID)})
	}

	if m.blkChainID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}CREATED BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.blkChainID)})
	}
	if m.vmName != "" {
		tb.Append([]string{formatter.F("{{dark-green}}VM NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmName)})
		tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmID)})
		tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmGenesisPath)})
	}

	tb.Render()
	return buf.String()
}
