// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/onsi/ginkgo/v2/formatter"
	"go.uber.org/zap"

	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
)

type Info struct {
	uri string

	balance uint64
	feeData *info.GetTxFeeResponse
	txFee   uint64

	key key.Key

	networkName string

	subnetIDType string
	subnetID     ids.ID

	nodeID ids.ShortID

	blockchainID  ids.ID
	chainName     string
	vmID          ids.ID
	vmGenesisPath string

	validateStart  time.Time
	validateEnd    time.Time
	validateWeight uint64
}

func InitClient(uri string, loadKey bool) (client.Client, *Info, error) {
	cli, err := client.New(client.Config{
		URI:            uri,
		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return nil, nil, err
	}
	txFee, err := cli.Info().Client().GetTxFee()
	if err != nil {
		return nil, nil, err
	}
	networkName, err := cli.Info().Client().GetNetworkName()
	if err != nil {
		return nil, nil, err
	}
	info := &Info{
		uri:         uri,
		feeData:     txFee,
		networkName: networkName,
	}
	if !loadKey {
		return cli, info, nil
	}

	info.key, err = key.Load(cli.NetworkID(), privKeyPath)
	if err != nil {
		return nil, nil, err
	}
	info.balance, err = cli.P().Balance(info.key)
	if err != nil {
		return nil, nil, err
	}
	return cli, info, nil
}

func CreateLogger() error {
	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	if err != nil {
		return err
	}
	_ = zap.ReplaceGlobals(logger)
	return nil
}

func (i *Info) CheckBalance() error {
	if i.balance < i.txFee {
		color.Outf("{{red}}insufficient funds to perform operation. get more at https://faucet.avax-test.network{{/}}\n")
		return fmt.Errorf("%w: on %s (expected=%d, have=%d)", ErrInsufficientFunds, i.key.P(), i.txFee, i.balance)
	}
	return nil
}

func BaseTableSetup(i *Info) (*bytes.Buffer, *tablewriter.Table) {
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
	if i.txFee != 0 {
		tb.Append([]string{formatter.F("{{red}}{{bold}}TX FEE{{/}}"), formatter.F("{{light-gray}}{{bold}}{{underline}}%s{{/}} $AVAX", txFees)})
	}
	tb.Append([]string{formatter.F("{{orange}}URI{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.uri)})
	tb.Append([]string{formatter.F("{{orange}}NETWORK NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", i.networkName)})
	return buf, tb
}
