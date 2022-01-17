// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/manifoldco/promptui"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newCreateSubnetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subnet",
		Short: "Creates a subnet",
		Long: `
Creates a subnet based on the configuration.

$ subnet-cli create subnet \
--private-key-path=.insecure.ewoq.key \
--uri=http://localhost:52250

`,
		RunE: createSubnetFunc,
	}

	cmd.PersistentFlags().BoolVar(&enablePrompt, "enable-prompt", true, "'true' to enable prompt mode")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logutil.DefaultLogLevel, "log level")
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", "", "private key file path")
	cmd.PersistentFlags().StringVar(&uri, "uri", "", "URI for avalanche network endpoints")
	cmd.PersistentFlags().DurationVar(&pollInterval, "poll-interval", time.Second, "interval to poll tx/blockchain status")
	cmd.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", 2*time.Minute, "request timeout")

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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	sid, _, err := cli.P().CreateSubnet(ctx, k, client.WithDryMode(true))
	cancel()
	if err != nil {
		return err
	}

	if nanoAvaxP < uint64(txFee.CreateSubnetTxFee) {
		return fmt.Errorf("insuffient fee on %s (expected=%d, have=%d)", k.P(), txFee.CreateSubnetTxFee, nanoAvaxP)
	}

	s := status{
		curPChainBalance: nanoAvaxP,
		txFee:            uint64(txFee.CreateSubnetTxFee),

		key: k,

		uri:         uri,
		networkName: networkName,

		pollInterval:   pollInterval,
		requestTimeout: requestTimeout,

		subnetIDType: "EXPECTED SUBNET ID",
		subnetID:     sid,
	}
	msg := s.Table(true)
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

	println()
	println()
	println()
	ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
	subnetID, took, err := cli.P().CreateSubnet(ctx, k, client.WithDryMode(false))
	cancel()
	if err != nil {
		return err
	}

	// TODO: call whitelisting
	color.Outf("{{magenta}}created subnet{{/}} %q {{light-gray}}(took %v){{/}}\n", s.subnetID, took)
	color.Outf("({{orange}}subnet must be whitelisted beforehand via{{/}} {{cyan}}{{bold}}--whitelisted-subnets{{/}} {{orange}}flag!{{/}})\n\n")

	s.subnetIDType = "CREATED SUBNET ID"
	s.subnetID = subnetID
	nanoAvaxP, err = cli.P().Balance(k)
	if err != nil {
		return err
	}
	s.curPChainBalance = nanoAvaxP
	fmt.Fprint(formatter.ColorableStdOut, s.Table(false))
	return nil
}
