// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package implements "add" sub-commands.
package add

import (
	"github.com/spf13/cobra"
)

func init() {
	cobra.EnablePrefixMatching = true
}

// NewCommand implements "subnet-cli add" command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Sub-commands for creating resources",
	}
	cmd.AddCommand(
		newAddValidatorCommand(),
	)
	return cmd
}
