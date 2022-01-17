// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/spf13/cobra"
)

func init() {
	cobra.EnablePrefixMatching = true
}

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
	cmd.PersistentFlags().StringVar(&privKeyPath, "private-key-path", "", "private key file path")
	return cmd
}

func createKeyFunc(cmd *cobra.Command, args []string) error {
	k, err := key.New(0, "generated")
	if err != nil {
		return err
	}
	// TODO: make sure not overwriting key
	if err := k.Save(privKeyPath); err != nil {
		return err
	}
	color.Outf("{{green}}created a new key %q{{/}}\n", privKeyPath)
	return nil
}
