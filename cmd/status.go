// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/spf13/cobra"
)

// NewCommand implements "subnet-cli status" command.
func StatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "status commands",
	}
	cmd.AddCommand(
		newStatusBlockchainCommand(),
	)
	cmd.PersistentFlags().StringVar(&privateURI, "private-uri", "", "URI for avalanche network endpoints")
	return cmd
}
