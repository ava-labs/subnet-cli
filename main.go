// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// "subnet-cli" implements quarkvm client operation interface.
package main

import (
	"fmt"
	"os"

	"github.com/ava-labs/subnet-cli/cmd/add"
	"github.com/ava-labs/subnet-cli/cmd/create"
	"github.com/ava-labs/subnet-cli/cmd/status"
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
		add.NewCommand(),
		status.NewCommand(),
	)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "subnet-cli failed %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
