// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	version = "v0.0.3"
)

// VersionCommand implements "subnet-cli version" command.
func VersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Prints the version and exits",
		Run:   printVersion,
	}
	return cmd
}

// printVersion and exit.
func printVersion(cmd *cobra.Command, args []string) {
	fmt.Println(version)
}
