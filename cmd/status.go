// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"time"

	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/spf13/cobra"
)

func init() {
	cobra.EnablePrefixMatching = true
}

// NewCommand implements "subnet-cli status" command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "status commands",
	}

	cmd.AddCommand(
		newStatusBlockchainCommand(),
	)

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logutil.DefaultLogLevel, "log level")
	cmd.PersistentFlags().StringVar(&uri, "uri", "", "URI for avalanche network endpoints")
	cmd.PersistentFlags().DurationVar(&pollInterval, "poll-interval", time.Second, "interval to poll tx/blockchain status")
	cmd.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", 2*time.Minute, "request timeout")

	return cmd
}
