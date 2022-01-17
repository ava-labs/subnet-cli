// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/spf13/cobra"
)

// NewAddCommand implements "subnet-cli add" command.
func NewAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Sub-commands for creating resources",
	}
	cmd.AddCommand(
		newAddValidatorCommand(),
	)
	return cmd
}
