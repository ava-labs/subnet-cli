// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package create

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	subnetIDs     string
	vmName        string
	vmIDs         string
	vmGenesisPath string
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
--vm-name=my-custom-vm \
--vm-id=tGas3T58KzdjLHhBDMnH2TvrddhqTji5iZAMZ3RXs2NLpSnhH \
--vm-genesis-path=.my-custom-vm.genesis

`,
		RunE: createBlockchainFunc,
	}

	cmd.PersistentFlags().BoolVar(&enablePrompt, "enable-prompt", true, "'true' to enable prompt mode")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logutil.DefaultLogLevel, "log level")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", "", "private key file path")
	cmd.PersistentFlags().StringVar(&uri, "uri", "", "URI for avalanche network endpoints")
	cmd.PersistentFlags().DurationVar(&pollInterval, "poll-interval", time.Second, "interval to poll tx/blockchain status")
	cmd.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", 2*time.Minute, "request timeout")

	cmd.PersistentFlags().StringVar(&subnetIDs, "subnet-id", "", "subnet ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmName, "vm-name", "", "VM name")
	cmd.PersistentFlags().StringVar(&vmIDs, "vm-id", "", "VM ID (must be formatted in ids.ID)")
	cmd.PersistentFlags().StringVar(&vmGenesisPath, "vm-genesis-path", "", "VM genesis file path")

	return cmd
}

func createBlockchainFunc(cmd *cobra.Command, args []string) error {
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

	// TODO: move base status init to shared package
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
	subnetID, err := ids.FromString(subnetIDs)
	if err != nil {
		return err
	}
	vmID, err := ids.FromString(vmIDs)
	if err != nil {
		return err
	}
	vmGenesisBytes, err := ioutil.ReadFile(vmGenesisPath)
	if err != nil {
		return err
	}

	if nanoAvaxP < uint64(txFee.CreateBlockchainTxFee) {
		return fmt.Errorf("insuffient fee on %s (expected=%d, have=%d)", k.P(), txFee.CreateSubnetTxFee, nanoAvaxP)
	}

	s := status{
		curPChainBalance: nanoAvaxP,
		txFee:            uint64(txFee.CreateBlockchainTxFee),

		key: k,

		uri:         uri,
		networkName: networkName,

		pollInterval:   pollInterval,
		requestTimeout: requestTimeout,

		subnetIDType: "SUBNET ID",
		subnetID:     subnetID,

		blkChainID:    ids.Empty,
		vmName:        vmName,
		vmID:          vmID,
		vmGenesisPath: vmGenesisPath,
	}
	msg := s.Table(true)
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
	blkChainID, took, err := cli.P().CreateBlockchain(
		ctx,
		k,
		s.subnetID,
		s.vmName,
		s.vmID,
		vmGenesisBytes,
		client.WithPoll(false),
	)
	cancel()
	if err != nil {
		return err
	}
	s.blkChainID = blkChainID
	color.Outf("{{magenta}}created blockchain{{/}} %q {{light-gray}}(took %v){{/}}\n\n", s.blkChainID, took)

	nanoAvaxP, err = cli.P().Balance(k)
	if err != nil {
		return err
	}
	s.curPChainBalance = nanoAvaxP
	fmt.Fprint(formatter.ColorableStdOut, s.Table(false))
	return nil
}
