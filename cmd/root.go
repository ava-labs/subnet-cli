// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:        "subnet-cli",
	Short:      "subnet-cli CLI",
	SuggestFor: []string{"subnet-cli", "subnetcli", "subnetctl"},
}

func init() {
	cobra.EnablePrefixMatching = true
}

func init() {
	rootCmd.AddCommand(
		create.NewCommand(),
		NewAddCommand(),
		status.NewCommand(),
	)
}

func Execute() error {
	return rootCmd.Execute()
}
