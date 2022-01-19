// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/spf13/cobra"
)

const (
	IDLen = 32
)

var (
	h bool
)

func newCreateVMIDCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "VMID [options] <identifier>",
		Short: "Creates a new encoded VMID from a string",
		RunE:  createVMIDFunc,
	}

	cmd.PersistentFlags().BoolVar(&h, "hash", false, "whether or not to hash the identifier argument")

	return cmd
}

func createVMIDFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected 1 argument but got %d", len(args))
	}

	identifier := []byte(args[0])
	var b []byte
	if h {
		b = hashing.ComputeHash256(identifier)
	} else {
		if len(identifier) > IDLen {
			return fmt.Errorf("non-hashed name must be <= 32 bytes, found %d", len(identifier))
		}
		b = make([]byte, IDLen)
		copy(b, identifier)
	}

	id, err := ids.ToID(b)
	if err != nil {
		return err
	}

	color.Outf("{{green}}created a new VMID %s from %s{{/}}\n", id.String(), args[0])
	return nil
}
