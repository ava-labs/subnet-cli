// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"os"

	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/spf13/cobra"
)

func newCreateKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key [options]",
		Short: "Generates a private key",
		Long: `
Generates a private key.

$ subnet-cli create key --private-key-path=.insecure.test.key

`,
		RunE: createKeyFunc,
	}
	return cmd
}

func createKeyFunc(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(privKeyPath); err == nil {
		color.Outf("{{red}}key already found at %q{{/}}\n", privKeyPath)
		return os.ErrExist
	}
	k, err := key.New(0, "generated")
	if err != nil {
		return err
	}
	if err := k.Save(privKeyPath); err != nil {
		return err
	}
	color.Outf("{{green}}created a new key %q{{/}}\n", privKeyPath)
	return nil
}
