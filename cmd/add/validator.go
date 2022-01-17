// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package add

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/olekukonko/tablewriter"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	enablePrompt bool
	logLevel     string

	privKeyPath string

	uri string

	pollInterval   time.Duration
	requestTimeout time.Duration

	subnetIDs string
	nodeIDs   string

	validateStarts string
	validateEnds   string
	validateWeight uint64
)

func newAddValidatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Adds a subnet to the validator",
		Long: `
Adds a subnet to the validator.

$ subnet-cli add validator \
--private-key-path=.insecure.ewoq.key \
--uri=http://localhost:52250 \
--subnet-id="24tZhrm8j8GCJRE9PomW8FaeqbgGS4UAQjJnqqn8pq5NwYSYV1" \
--node-id="NodeID-4B4rc5vdD1758JSBYL1xyvE5NHGzz6xzH" \
--validate-weight=1000

`,
		RunE: createSubnetFunc,
	}

	cmd.PersistentFlags().BoolVar(&enablePrompt, "enable-prompt", true, "'true' to enable prompt mode")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logutil.DefaultLogLevel, "log level")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", "", "private key file path")
	cmd.PersistentFlags().StringVar(&uri, "uri", "", "URI for avalanche network endpoints")
	cmd.PersistentFlags().DurationVar(&pollInterval, "poll-interval", time.Second, "interval to poll tx/blockchain status")
	cmd.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", 2*time.Minute, "request timeout")

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&nodeIDs, "node-id", "", "node ID (must be formatted in ids.ID)")

	start := time.Now().Add(30 * time.Second)
	end := start.Add(2 * 24 * time.Hour)
	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint64Var(&validateWeight, "validate-weight", 1000, "validate weight")

	return cmd
}

func createSubnetFunc(cmd *cobra.Command, args []string) error {
	color.Outf("\n\n{{blue}}Setting up the configuration!{{/}}\n\n")

	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	if err != nil {
		log.Fatalf("failed to build global logger, %v", err)
	}
	_ = zap.ReplaceGlobals(logger)

	cli, err := client.New(client.Config{
		URI:            uri,
		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return err
	}
	k, err := key.Load(cli.NetworkID(), privKeyPath)
	if err != nil {
		return err
	}

	nanoAvaxP, err := cli.P().Balance(k)
	if err != nil {
		return err
	}
	txFee, err := cli.Info().Client().GetTxFee()
	if err != nil {
		return err
	}
	networkName, err := cli.Info().Client().GetNetworkName()
	if err != nil {
		return err
	}
	nodeID, err := ids.ShortFromPrefixedString(nodeIDs, constants.NodeIDPrefix)
	if err != nil {
		return err
	}
	subnetID, err := ids.FromString(subnetIDs)
	if err != nil {
		return err
	}
	validateStart, err := time.Parse(time.RFC3339, validateStarts)
	if err != nil {
		return err
	}
	validateEnd, err := time.Parse(time.RFC3339, validateEnds)
	if err != nil {
		return err
	}

	if nanoAvaxP < uint64(txFee.TxFee) {
		return fmt.Errorf("insuffient fee on %s (expected=%d, have=%d)", k.P(), txFee.TxFee, nanoAvaxP)
	}

	s := status{
		curPChainBalance:      nanoAvaxP,
		txFee:                 uint64(txFee.TxFee),
		subnetTxFee:           uint64(txFee.CreateSubnetTxFee),
		createBlockchainTxFee: uint64(txFee.CreateBlockchainTxFee),
		afterPChainBalance:    nanoAvaxP - uint64(txFee.TxFee),

		key: k,

		uri:         uri,
		nodeID:      nodeID,
		networkName: networkName,

		pollInterval:   pollInterval,
		requestTimeout: requestTimeout,

		subnetID:       subnetID,
		validateStart:  validateStart,
		validateEnd:    validateEnd,
		validateWeight: validateWeight,
	}
	msg := s.Table(true)
	if enablePrompt {
		msg = formatter.F("\n{{blue}}{{bold}}Ready to add subnet validator, should we continue?{{/}}\n") + msg
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
	took, err := cli.P().AddSubnetValidator(
		ctx,
		k,
		s.subnetID,
		s.nodeID,
		validateStart,
		validateEnd,
		validateWeight,
	)
	cancel()
	if err != nil {
		return err
	}
	color.Outf("{{magenta}}added subnet to validator{{/}} %q {{light-gray}}(took %v){{/}}\n\n", s.subnetID, took)

	nanoAvaxP, err = cli.P().Balance(k)
	if err != nil {
		return err
	}
	s.curPChainBalance = nanoAvaxP
	fmt.Fprint(formatter.ColorableStdOut, s.Table(false))
	return nil
}

type status struct {
	curPChainBalance      uint64
	txFee                 uint64
	subnetTxFee           uint64
	createBlockchainTxFee uint64
	afterPChainBalance    uint64

	key key.Key

	uri         string
	nodeID      ids.ShortID
	networkName string

	pollInterval   time.Duration
	requestTimeout time.Duration

	subnetID       ids.ID
	validateStart  time.Time
	validateEnd    time.Time
	validateWeight uint64
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
	tb.Append([]string{formatter.F("{{orange}}NODE ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.nodeID)})
	tb.Append([]string{formatter.F("{{orange}}NETWORK NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.networkName)})
	tb.Append([]string{formatter.F("{{orange}}POLL INTERVAL{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.pollInterval)})
	tb.Append([]string{formatter.F("{{orange}}REQUEST TIMEOUT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.requestTimeout)})

	tb.Append([]string{formatter.F("{{blue}}SUBNET ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.subnetID)})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.validateStart.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.validateEnd.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(m.validateWeight)))})

	tb.Render()
	return buf.String()
}
