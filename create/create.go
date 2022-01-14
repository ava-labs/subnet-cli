// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package create implements "create" commands.
package create

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
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
	formatter "github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func init() {
	cobra.EnablePrefixMatching = true
}

var (
	enablePrompt   bool
	dryMode        bool
	privKeyPath    string
	logLevel       string
	uri            string
	networkID      uint32
	pollInterval   time.Duration
	requestTimeout time.Duration

	validateStarts string
	validateEnds   string
	validateWeight uint64

	vmName        string
	vmID          string
	vmGenesisPath string
)

// NewCommand implements "subnet-cli create" command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [options]",
		Short: "Creates a subnet and blockchain for the custom VM.",
		Long: `
Creates a subnet based on the configuration.

# try with the pre-funded ewoq key
# use 1337 for network-runner local cluster
$ subnet-cli create \
--enable-prompt=true \
--dry-mode=false \
--log-level=debug \
--private-key-path=.insecure.ewoq.key \
--uri=http://localhost:49738 \
--network-id=1337 \
--poll-interval=1s \
--request-timeout=2m \
--validate-start="2022-01-02T15:04:05Z07:00" \
--validate-end="2022-01-02T19:04:05Z07:00" \
--validate-weight=1000 \
--vm-name=my-custom-vm \
--vm-id=tGas3T58KzdjLHhBDMnH2TvrddhqTji5iZAMZ3RXs2NLpSnhH \
--vm-genesis-path=.my-custom-vm.genesis

`,
		RunE: createFunc,
	}

	cmd.PersistentFlags().BoolVar(&enablePrompt, "enable-prompt", true, "'true' to enable prompt mode")
	cmd.PersistentFlags().BoolVar(&dryMode, "dry-mode", true, "'true' to enable dry mode, must be 'false' to create blockchain resources")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logutil.DefaultLogLevel, "log level")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", "", "private key file path")
	cmd.PersistentFlags().StringVar(&uri, "uri", "", "URI for avalanche network endpoints")
	cmd.PersistentFlags().Uint32Var(&networkID, "network-id", 0, "network ID defined in the genesis file")
	cmd.PersistentFlags().DurationVar(&pollInterval, "poll-interval", time.Second, "interval to poll tx/blockchain status")
	cmd.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", 2*time.Minute, "request timeout")

	start := time.Now().Add(30 * time.Second)
	end := start.Add(2 * 24 * time.Hour)

	cmd.PersistentFlags().StringVar(&validateStarts, "validate-start", start.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().StringVar(&validateEnds, "validate-end", end.Format(time.RFC3339), "validate start timestamp in RFC3339 format")
	cmd.PersistentFlags().Uint64Var(&validateWeight, "validate-weight", 1000, "validate weight")

	cmd.PersistentFlags().StringVar(&vmName, "vm-name", "", "VM name")
	cmd.PersistentFlags().StringVar(&vmID, "vm-id", "", "VM ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmGenesisPath, "vm-genesis-path", "", "VM genesis file path")

	return cmd
}

func createFunc(cmd *cobra.Command, args []string) error {
	color.Outf("\n\n{{blue}}Setting up the configuration!{{/}}\n\n")

	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	if err != nil {
		log.Fatalf("failed to build global logger, %v", err)
	}
	_ = zap.ReplaceGlobals(logger)

	cli, err := client.New(client.Config{
		RootCtx: context.Background(),
		URI:     uri,

		NetworkID: networkID,

		AssetID:  ids.Empty, // to auto-fetch by client
		PChainID: ids.Empty, // to auto-fetch by client
		XChainID: ids.Empty, // to auto-fetch by client

		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return err
	}

	k, err := key.Load(privKeyPath)
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

	nodeIDs, err := cli.Info().Client().GetNodeID()
	if err != nil {
		return err
	}
	nodeID, err := ids.ShortFromPrefixedString(nodeIDs, constants.NodeIDPrefix)
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

	vid, err := ids.FromString(vmID)
	if err != nil {
		return err
	}
	vmGenesisBytes, err := ioutil.ReadFile(vmGenesisPath)
	if err != nil {
		return err
	}

	expectedSubnetID, _, err := cli.P().CreateSubnet(k, client.WithDryMode(true))
	if err != nil {
		return err
	}

	m := status{
		curPChainBalance:      nanoAvaxP,
		txFee:                 uint64(txFee.TxFee),
		subnetTxFee:           uint64(txFee.CreateSubnetTxFee),
		createBlockchainTxFee: uint64(txFee.CreateBlockchainTxFee),

		key: k,

		uri:    uri,
		nodeID: nodeID,

		networkName: networkName,
		networkID:   networkID,

		pollInterval:   pollInterval,
		requestTimeout: requestTimeout,

		validateStart:  validateStart,
		validateEnd:    validateEnd,
		validateWeight: validateWeight,

		vmName:        vmName,
		vmID:          vid,
		vmGenesisPath: vmGenesisPath,

		subnetIDType: "EXPECTED SUBNET ID",
		subnetID:     expectedSubnetID,
	}
	m.afterPChainBalance = m.curPChainBalance - m.txFee - m.subnetTxFee - m.createBlockchainTxFee
	msg := m.Table(true)
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
	if dryMode {
		return nil
	}

	println()
	println()
	println()
	returned, took, err := cli.P().CreateSubnet(k, client.WithDryMode(false))
	if err != nil {
		return err
	}
	m.subnetIDType = "CREATED SUBNET ID"
	m.subnetID = returned

	// TODO: call whitelisting
	color.Outf("{{magenta}}created subnet{{/}} %q {{light-gray}}(took %v){{/}}\n", m.subnetID, took)
	color.Outf("({{orange}}subnet must be whitelisted beforehand via{{/}} {{cyan}}{{bold}}--whitelisted-subnets{{/}} {{orange}}flag!{{/}})\n\n")

	took, err = cli.P().AddSubnetValidator(
		k,
		m.subnetID,
		m.nodeID,
		validateStart,
		validateEnd,
		validateWeight,
	)
	if err != nil {
		return err
	}
	color.Outf("{{magenta}}added subnet to validator{{/}} %q {{light-gray}}(took %v){{/}}\n\n", m.subnetID, took)

	m.blkChainID, took, err = cli.P().CreateBlockchain(
		k,
		m.subnetID,
		m.vmName,
		m.vmID,
		vmGenesisBytes,
	)
	if err != nil {
		return err
	}
	color.Outf("{{magenta}}created blockchain{{/}} %q {{light-gray}}(took %v){{/}}\n\n", m.blkChainID, took)

	nanoAvaxP, err = cli.P().Balance(k)
	if err != nil {
		return err
	}
	m.curPChainBalance = nanoAvaxP
	m.afterPChainBalance = m.curPChainBalance - m.txFee - m.subnetTxFee - m.createBlockchainTxFee

	fmt.Fprint(formatter.ColorableStdOut, m.Table(false))
	return nil
}

type status struct {
	curPChainBalance      uint64
	txFee                 uint64
	subnetTxFee           uint64
	createBlockchainTxFee uint64
	afterPChainBalance    uint64

	key key.Key

	uri    string
	nodeID ids.ShortID

	networkName string
	networkID   uint32

	pollInterval   time.Duration
	requestTimeout time.Duration

	validateStart  time.Time
	validateEnd    time.Time
	validateWeight uint64

	vmName        string
	vmID          ids.ID
	vmGenesisPath string

	subnetIDType string
	subnetID     ids.ID
	blkChainID   ids.ID
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
	tb.Append([]string{formatter.F("{{orange}}NETWORK ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%d{{/}}", m.networkID)})
	tb.Append([]string{formatter.F("{{orange}}NETWORK NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.networkName)})
	tb.Append([]string{formatter.F("{{orange}}POLL INTERVAL{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.pollInterval)})
	tb.Append([]string{formatter.F("{{orange}}REQUEST TIMEOUT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.requestTimeout)})

	tb.Append([]string{formatter.F("{{magenta}}VALIDATE START{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.validateStart.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE END{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.validateEnd.Format(time.RFC3339))})
	tb.Append([]string{formatter.F("{{magenta}}VALIDATE WEIGHT{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", humanize.Comma(int64(m.validateWeight)))})

	tb.Append([]string{formatter.F("{{dark-green}}VM NAME{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmName)})
	tb.Append([]string{formatter.F("{{dark-green}}VM ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmID)})
	tb.Append([]string{formatter.F("{{dark-green}}VM GENESIS PATH{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.vmGenesisPath)})

	if m.subnetID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}%s{{/}}", m.subnetIDType), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.subnetID)})
	}
	if m.blkChainID != ids.Empty {
		tb.Append([]string{formatter.F("{{blue}}CREATED BLOCKCHAIN ID{{/}}"), formatter.F("{{light-gray}}{{bold}}%s{{/}}", m.blkChainID)})
	}

	tb.Render()
	return buf.String()
}
