// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/coreth/plugin/evm"
)

var (
	xCodecManager codec.Manager
	pCodecManager codec.Manager
	cCodecManager codec.Manager
)

func init() {
	xc := linearcodec.NewDefault()
	xCodecManager = codec.NewDefaultManager()
	errs := wrappers.Errs{}
	errs.Add(
		xc.RegisterType(&avm.BaseTx{}),
		xc.RegisterType(&avm.CreateAssetTx{}),
		xc.RegisterType(&avm.OperationTx{}),
		xc.RegisterType(&avm.ImportTx{}),
		xc.RegisterType(&avm.ExportTx{}),
		xc.RegisterType(&secp256k1fx.TransferInput{}),
		xc.RegisterType(&secp256k1fx.MintOutput{}),
		xc.RegisterType(&secp256k1fx.TransferOutput{}),
		xc.RegisterType(&secp256k1fx.MintOperation{}),
		xc.RegisterType(&secp256k1fx.Credential{}),
		xCodecManager.RegisterCodec(0, xc),
	)
	if errs.Errored() {
		panic(errs.Err)
	}

	pc := linearcodec.NewDefault()
	pCodecManager = codec.NewDefaultManager()
	errs = wrappers.Errs{}
	errs.Add(
		pc.RegisterType(&platformvm.ProposalBlock{}),
		pc.RegisterType(&platformvm.AbortBlock{}),
		pc.RegisterType(&platformvm.CommitBlock{}),
		pc.RegisterType(&platformvm.StandardBlock{}),
		pc.RegisterType(&platformvm.AtomicBlock{}),
		pc.RegisterType(&secp256k1fx.TransferInput{}),
		pc.RegisterType(&secp256k1fx.MintOutput{}),
		pc.RegisterType(&secp256k1fx.TransferOutput{}),
		pc.RegisterType(&secp256k1fx.MintOperation{}),
		pc.RegisterType(&secp256k1fx.Credential{}),
		pc.RegisterType(&secp256k1fx.Input{}),
		pc.RegisterType(&secp256k1fx.OutputOwners{}),
		pc.RegisterType(&platformvm.UnsignedAddValidatorTx{}),
		pc.RegisterType(&platformvm.UnsignedAddSubnetValidatorTx{}),
		pc.RegisterType(&platformvm.UnsignedAddDelegatorTx{}),
		pc.RegisterType(&platformvm.UnsignedCreateChainTx{}),
		pc.RegisterType(&platformvm.UnsignedCreateSubnetTx{}),
		pc.RegisterType(&platformvm.UnsignedImportTx{}),
		pc.RegisterType(&platformvm.UnsignedExportTx{}),
		pc.RegisterType(&platformvm.UnsignedAdvanceTimeTx{}),
		pc.RegisterType(&platformvm.UnsignedRewardValidatorTx{}),
		pc.RegisterType(&platformvm.StakeableLockIn{}),
		pc.RegisterType(&platformvm.StakeableLockOut{}),
		pCodecManager.RegisterCodec(0, pc),
	)
	if errs.Errored() {
		panic(errs.Err)
	}

	cc := linearcodec.NewDefault()
	cCodecManager = codec.NewDefaultManager()
	errs = wrappers.Errs{}
	errs.Add(
		cc.RegisterType(&evm.UnsignedImportTx{}),
		cc.RegisterType(&evm.UnsignedExportTx{}),
	)
	cc.SkipRegistrations(3)
	errs.Add(
		cc.RegisterType(&secp256k1fx.TransferInput{}),
		cc.RegisterType(&secp256k1fx.MintOutput{}),
		cc.RegisterType(&secp256k1fx.TransferOutput{}),
		cc.RegisterType(&secp256k1fx.MintOperation{}),
		cc.RegisterType(&secp256k1fx.Credential{}),
		cc.RegisterType(&secp256k1fx.Input{}),
		cc.RegisterType(&secp256k1fx.OutputOwners{}),
		cCodecManager.RegisterCodec(0, cc),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
}
