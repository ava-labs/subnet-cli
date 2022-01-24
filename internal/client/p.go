// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	api_info "github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	internal_avax "github.com/ava-labs/subnet-cli/internal/avax"
	"github.com/ava-labs/subnet-cli/internal/key"
	internal_platformvm "github.com/ava-labs/subnet-cli/internal/platformvm"
	"go.uber.org/zap"
)

var (
	ErrInsufficientBalanceForGasFee = errors.New("insufficient balance for gas")
	ErrUnexpectedSubnetID           = errors.New("unexpected subnet ID")

	// ref. "vms.platformvm".
	ErrWrongTxType   = errors.New("wrong transaction type")
	ErrUnknownOwners = errors.New("unknown owners")
	ErrCantSign      = errors.New("can't sign")
)

type P interface {
	Client() platformvm.Client
	Checker() internal_platformvm.Checker
	Balance(key key.Key) (uint64, error)
	AddValidator(
		ctx context.Context,
		k key.Key,
		nodeID ids.ShortID,
		start time.Time,
		end time.Time,
		amount uint64,
		opts ...OpOption,
	) (took time.Duration, err error)
	CreateSubnet(
		ctx context.Context,
		key key.Key,
		opts ...OpOption,
	) (subnetID ids.ID, took time.Duration, err error)
	AddSubnetValidator(
		ctx context.Context,
		k key.Key,
		subnetID ids.ID,
		nodeID ids.ShortID,
		start time.Time,
		end time.Time,
		weight uint64,
		opts ...OpOption,
	) (took time.Duration, err error)
	CreateBlockchain(
		ctx context.Context,
		key key.Key,
		subnetID ids.ID,
		chainName string,
		vmID ids.ID,
		vmGenesis []byte,
		opts ...OpOption,
	) (blkChainID ids.ID, took time.Duration, err error)
}

type p struct {
	cfg       Config
	networkID uint32
	assetID   ids.ID
	pChainID  ids.ID

	cli     platformvm.Client
	info    api_info.Client
	checker internal_platformvm.Checker
}

func (pc *p) Client() platformvm.Client            { return pc.cli }
func (pc *p) Checker() internal_platformvm.Checker { return pc.checker }

func (pc *p) Balance(key key.Key) (uint64, error) {
	pb, err := pc.cli.GetBalance(key.P())
	if err != nil {
		return 0, err
	}
	return uint64(pb.Balance), nil
}

// ref. "platformvm.VM.newCreateSubnetTx".
func (pc *p) CreateSubnet(
	ctx context.Context,
	k key.Key,
	opts ...OpOption,
) (subnetID ids.ID, took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	fi, err := pc.info.GetTxFee()
	if err != nil {
		return ids.Empty, 0, err
	}
	createSubnetTxFee := uint64(fi.CreateSubnetTxFee)

	zap.L().Info("creating subnet",
		zap.Bool("dryMode", ret.dryMode),
		zap.String("assetId", pc.assetID.String()),
		zap.Uint64("createSubnetTxFee", createSubnetTxFee),
	)
	ins, outs, signers, err := pc.transact(k, createSubnetTxFee)
	if err != nil {
		return ids.Empty, 0, err
	}

	utx := &platformvm.UnsignedCreateSubnetTx{
		BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         outs,
		}},
		Owner: &secp256k1fx.OutputOwners{
			// [threshold] of [ownerAddrs] needed to manage this subnet
			Threshold: 1,

			// address to send change to, if there is any,
			// control addresses for the new subnet
			Addrs: []ids.ShortID{k.Key().PublicKey().Address()},
		},
	}
	pTx := &platformvm.Tx{
		UnsignedTx: utx,
	}
	if err := pTx.Sign(pCodecManager, signers); err != nil {
		return ids.Empty, 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return ids.Empty, 0, err
	}

	// subnet tx ID is the subnet ID based on ins/outs
	subnetID = pTx.ID()
	if ret.dryMode {
		return subnetID, 0, nil
	}

	txID, err := pc.cli.IssueTx(pTx.Bytes())
	if err != nil {
		return subnetID, 0, fmt.Errorf("failed to issue tx: %w", err)
	}
	if txID != subnetID {
		return subnetID, 0, ErrUnexpectedSubnetID
	}

	took, err = pc.checker.PollSubnet(ctx, txID)
	return txID, took, err
}

// ref. "platformvm.VM.newAddValidatorTx".
// WIP
func (pc *p) AddValidator(
	ctx context.Context,
	k key.Key,
	nodeID ids.ShortID,
	start time.Time,
	end time.Time,
	amount uint64,
	opts ...OpOption,
) (took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	if nodeID == ids.ShortEmpty {
		return 0, ErrEmptyID
	}

	zap.L().Info("adding validator",
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Uint64("amount", amount),
	)
	// TODO: replace
	ins, outs, signers, err := pc.transact(k, 0)
	if err != nil {
		return 0, err
	}

	utx := &platformvm.UnsignedAddValidatorTx{
		BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         outs,
		}},
		Validator: platformvm.Validator{
			NodeID: nodeID,
			Start:  uint64(start.Unix()),
			End:    uint64(end.Unix()),
			Wght:   amount,
		},
	}
	pTx := &platformvm.Tx{
		UnsignedTx: utx,
	}
	if err := pTx.Sign(pCodecManager, signers); err != nil {
		return 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return 0, err
	}
	txID, err := pc.cli.IssueTx(pTx.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to issue tx: %w", err)
	}

	return pc.checker.PollTx(ctx, txID, platformvm.Committed)
}

// ref. "platformvm.VM.newAddSubnetValidatorTx".
func (pc *p) AddSubnetValidator(
	ctx context.Context,
	k key.Key,
	subnetID ids.ID,
	nodeID ids.ShortID,
	start time.Time,
	end time.Time,
	weight uint64,
	opts ...OpOption,
) (took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	if subnetID == ids.Empty {
		// same as "ErrNamedSubnetCantBePrimary"
		// in case "subnetID == constants.PrimaryNetworkID"
		return 0, ErrEmptyID
	}
	if nodeID == ids.ShortEmpty {
		return 0, ErrEmptyID
	}

	fi, err := pc.info.GetTxFee()
	if err != nil {
		return 0, err
	}
	txFee := uint64(fi.TxFee)

	zap.L().Info("adding subnet validator",
		zap.String("subnetId", subnetID.String()),
		zap.Uint64("txFee", txFee),
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Uint64("weight", weight),
	)
	ins, outs, signers, err := pc.transact(k, txFee)
	if err != nil {
		return 0, err
	}
	subnetAuth, subnetSigners, err := pc.authorize(k, subnetID)
	if err != nil {
		return 0, err
	}
	signers = append(signers, subnetSigners)

	utx := &platformvm.UnsignedAddSubnetValidatorTx{
		BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         outs,
		}},
		Validator: platformvm.SubnetValidator{
			Validator: platformvm.Validator{
				NodeID: nodeID,
				Start:  uint64(start.Unix()),
				End:    uint64(end.Unix()),
				Wght:   weight,
			},
			Subnet: subnetID,
		},
		SubnetAuth: subnetAuth,
	}
	pTx := &platformvm.Tx{
		UnsignedTx: utx,
	}
	if err := pTx.Sign(pCodecManager, signers); err != nil {
		return 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return 0, err
	}
	txID, err := pc.cli.IssueTx(pTx.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to issue tx: %w", err)
	}

	return pc.checker.PollTx(ctx, txID, platformvm.Committed)
}

// ref. "platformvm.VM.newCreateChainTx".
func (pc *p) CreateBlockchain(
	ctx context.Context,
	k key.Key,
	subnetID ids.ID,
	chainName string,
	vmID ids.ID,
	vmGenesis []byte,
	opts ...OpOption,
) (blkChainID ids.ID, took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	if subnetID == ids.Empty {
		return ids.Empty, 0, ErrEmptyID
	}
	if vmID == ids.Empty {
		return ids.Empty, 0, ErrEmptyID
	}

	fi, err := pc.info.GetTxFee()
	if err != nil {
		return ids.Empty, 0, err
	}
	createBlkChainTxFee := uint64(fi.CreateBlockchainTxFee)

	now := time.Now()
	zap.L().Info("creating blockchain",
		zap.String("subnetId", subnetID.String()),
		zap.String("chainName", chainName),
		zap.String("vmId", vmID.String()),
		zap.Uint64("createBlockchainTxFee", createBlkChainTxFee),
	)
	ins, outs, signers, err := pc.transact(k, createBlkChainTxFee)
	if err != nil {
		return ids.Empty, 0, err
	}
	subnetAuth, subnetSigners, err := pc.authorize(k, subnetID)
	if err != nil {
		return ids.Empty, 0, err
	}
	signers = append(signers, subnetSigners)

	utx := &platformvm.UnsignedCreateChainTx{
		BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         outs,
		}},
		SubnetID:    subnetID,
		ChainName:   chainName,
		VMID:        vmID,
		FxIDs:       nil,
		GenesisData: vmGenesis,
		SubnetAuth:  subnetAuth,
	}
	pTx := &platformvm.Tx{
		UnsignedTx: utx,
	}
	if err := pTx.Sign(pCodecManager, signers); err != nil {
		return ids.Empty, 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return ids.Empty, 0, err
	}
	blkChainID, err = pc.cli.IssueTx(pTx.Bytes())
	if err != nil {
		return ids.Empty, 0, fmt.Errorf("failed to issue tx: %w", err)
	}

	took = time.Since(now)
	if ret.poll {
		var bTook time.Duration
		bTook, err = pc.checker.PollBlockchain(
			ctx,
			internal_platformvm.WithSubnetID(subnetID),
			internal_platformvm.WithBlockchainID(blkChainID),
			internal_platformvm.WithBlockchainStatus(platformvm.Validating),
			internal_platformvm.WithCheckBlockchainBootstrapped(pc.info),
		)
		took += bTook
	}
	return blkChainID, took, err
}

type Op struct {
	dryMode bool
	poll    bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithDryMode(b bool) OpOption {
	return func(op *Op) {
		op.dryMode = b
	}
}

func WithPoll(b bool) OpOption {
	return func(op *Op) {
		op.poll = b
	}
}

// ref. "platformvm.VM.stake".
func (pc *p) transact(k key.Key, fee uint64) (
	ins []*avax.TransferableInput,
	outs []*avax.TransferableOutput,
	signers [][]*crypto.PrivateKeySECP256K1R,
	err error,
) {
	ubs, _, err := pc.cli.GetAtomicUTXOs([]string{k.P()}, "", 100, "", "")
	if err != nil {
		return nil, nil, nil, err
	}

	now := uint64(time.Now().Unix())

	ins = make([]*avax.TransferableInput, 0)
	outs = make([]*avax.TransferableOutput, 0)
	signers = make([][]*crypto.PrivateKeySECP256K1R, 0)

	amountBurned := uint64(0)
	for _, ub := range ubs {
		// have burned more AVAX then we need to
		// no need to consume more AVAX
		if amountBurned >= fee {
			break
		}

		utxo, err := internal_avax.ParseUTXO(ub, pCodecManager)
		if err != nil {
			return nil, nil, nil, err
		}
		// assume "AssetID" is set to "AVAX" asset ID
		if utxo.AssetID() != pc.assetID {
			continue
		}

		out := utxo.Out
		inner, ok := out.(*platformvm.StakeableLockOut)
		if ok {
			if inner.Locktime > now {
				// output currently locked, can't be burned
				// skip for next UTXO
				continue
			}
			utxo.Out = inner.TransferableOut
		}
		_, inputs, inputSigners := k.Spends([]*avax.UTXO{utxo}, key.WithTime(now))
		if len(inputs) == 0 {
			// cannot spend this UTXO, skip to try next one
			continue
		}
		in := inputs[0]

		// initially the full value of the input
		remainingValue := in.In.Amount()

		// burn any value that should be burned
		amountToBurn := math.Min64(
			fee-amountBurned, // amount we still need to burn
			remainingValue,   // amount available to burn
		)
		amountBurned += amountToBurn
		remainingValue -= amountToBurn

		if remainingValue > 0 {
			// input had extra value, so some of it must be returned
			outs = append(outs, &avax.TransferableOutput{
				Asset: avax.Asset{ID: pc.assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: remainingValue,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,

						// address to send change to, if there is any
						Addrs: []ids.ShortID{k.Key().PublicKey().Address()},
					},
				},
			})
		}

		// add the input to the consumed inputs
		ins = append(ins, in)
		signers = append(signers, inputSigners...)
	}
	if amountBurned > 0 && amountBurned < fee {
		return nil, nil, nil, ErrInsufficientBalanceForGasFee
	}

	avax.SortTransferableInputsWithSigners(ins, signers) // sort inputs and keys
	avax.SortTransferableOutputs(outs, pCodecManager)    // sort outputs
	return ins, outs, signers, nil
}

func (pc *p) stake(k key.Key, amount uint64, fee uint64) (
	ins []*avax.TransferableInput,
	outs []*avax.TransferableOutput,
	stakedOuts []*avax.TransferableOutput,
	signers [][]*crypto.PrivateKeySECP256K1R,
	err error,
) {
	ubs, _, err := pc.cli.GetAtomicUTXOs([]string{k.P()}, "", 100, "", "")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Minimum time this transaction will be issued at
	now := uint64(time.Now().Unix())

	ins = []*avax.TransferableInput{}
	outs = []*avax.TransferableOutput{}
	stakedOuts = []*avax.TransferableOutput{}
	signers = [][]*crypto.PrivateKeySECP256K1R{}

	// Amount of AVAX that has been staked
	amountStaked := uint64(0)

	// Consume locked UTXOs
	for _, ub := range ubs {
		// If we have consumed more AVAX than we are trying to stake, then we
		// have no need to consume more locked AVAX
		if amountStaked >= amount {
			break
		}

		utxo, err := internal_avax.ParseUTXO(ub, pCodecManager)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		// assume "AssetID" is set to "AVAX" asset ID
		if utxo.AssetID() != pc.assetID {
			continue
		}

		out := utxo.Out
		inner, ok := out.(*platformvm.StakeableLockOut)
		if ok {
			if inner.Locktime > now {
				// output currently locked, can't be burned
				// skip for next UTXO
				continue
			}
			utxo.Out = inner.TransferableOut
		}

		_, inputs, inputSigners := k.Spends([]*avax.UTXO{utxo}, key.WithTime(now))
		if len(inputs) == 0 {
			// cannot spend this UTXO, skip to try next one
			continue
		}
		in := inputs[0]

		// The remaining value is initially the full value of the input
		remainingValue := in.In.Amount()

		// Stake any value that should be staked
		amountToStake := math.Min64(
			amount-amountStaked, // Amount we still need to stake
			remainingValue,      // Amount available to stake
		)
		amountStaked += amountToStake
		remainingValue -= amountToStake

		// Add the input to the consumed inputs
		ins = append(ins, in)

		// Add the output to the staked outputs
		stakedOuts = append(stakedOuts, &avax.TransferableOutput{
			Asset: avax.Asset{ID: pc.assetID},
			Out: &platformvm.StakeableLockOut{
				Locktime: in.Locktime,
				TransferableOut: &secp256k1fx.TransferOutput{
					Amt:          amountToStake,
					OutputOwners: inner.OutputOwners,
				},
			},
		})

		if remainingValue > 0 {
			// This input provided more value than was needed to be locked.
			// Some of it must be returned
			returnedOuts = append(returnedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: vm.ctx.AVAXAssetID},
				Out: &StakeableLockOut{
					Locktime: out.Locktime,
					TransferableOut: &secp256k1fx.TransferOutput{
						Amt:          remainingValue,
						OutputOwners: inner.OutputOwners,
					},
				},
			})
		}

		// Add the signers needed for this input to the set of signers
		signers = append(signers, inSigners)
	}

	// Amount of AVAX that has been burned
	amountBurned := uint64(0)

	for _, utxo := range utxos {
		// If we have consumed more AVAX than we are trying to stake, and we
		// have burned more AVAX then we need to, then we have no need to
		// consume more AVAX
		if amountBurned >= fee && amountStaked >= amount {
			break
		}

		if assetID := utxo.AssetID(); assetID != vm.ctx.AVAXAssetID {
			continue // We only care about burning AVAX, so ignore other assets
		}

		out := utxo.Out
		inner, ok := out.(*StakeableLockOut)
		if ok {
			if inner.Locktime > now {
				// This output is currently locked, so this output can't be
				// burned. Additionally, it may have already been consumed
				// above. Regardless, we skip to the next UTXO
				continue
			}
			out = inner.TransferableOut
		}

		inIntf, inSigners, err := kc.Spend(out, now)
		if err != nil {
			// We couldn't spend this UTXO, so we skip to the next one
			continue
		}
		in, ok := inIntf.(avax.TransferableIn)
		if !ok {
			// Because we only use the secp Fx right now, this should never
			// happen
			continue
		}

		// The remaining value is initially the full value of the input
		remainingValue := in.Amount()

		// Burn any value that should be burned
		amountToBurn := math.Min64(
			fee-amountBurned, // Amount we still need to burn
			remainingValue,   // Amount available to burn
		)
		amountBurned += amountToBurn
		remainingValue -= amountToBurn

		// Stake any value that should be staked
		amountToStake := math.Min64(
			amount-amountStaked, // Amount we still need to stake
			remainingValue,      // Amount available to stake
		)
		amountStaked += amountToStake
		remainingValue -= amountToStake

		// Add the input to the consumed inputs
		ins = append(ins, &avax.TransferableInput{
			UTXOID: utxo.UTXOID,
			Asset:  avax.Asset{ID: vm.ctx.AVAXAssetID},
			In:     in,
		})

		if amountToStake > 0 {
			// Some of this input was put for staking
			stakedOuts = append(stakedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: vm.ctx.AVAXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: amountToStake,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{changeAddr},
					},
				},
			})
		}

		if remainingValue > 0 {
			// This input had extra value, so some of it must be returned
			returnedOuts = append(returnedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: vm.ctx.AVAXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: remainingValue,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{changeAddr},
					},
				},
			})
		}

		// Add the signers needed for this input to the set of signers
		signers = append(signers, inSigners)
	}

	if amountBurned < fee || amountStaked < amount {
		return nil, nil, nil, nil, fmt.Errorf(
			"provided keys have balance (unlocked, locked) (%d, %d) but need (%d, %d)",
			amountBurned, amountStaked, fee, amount)
	}

	avax.SortTransferableInputsWithSigners(ins, signers) // sort inputs and keys
	avax.SortTransferableOutputs(returnedOuts, Codec)    // sort outputs
	avax.SortTransferableOutputs(stakedOuts, Codec)      // sort outputs

	return ins, returnedOuts, stakedOuts, signers, nil
}

// ref. "platformvm.VM.authorize".
func (pc *p) authorize(k key.Key, subnetID ids.ID) (
	auth verify.Verifiable, // input that names owners
	signers []*crypto.PrivateKeySECP256K1R, // keys that prove ownership
	err error,
) {
	tb, err := pc.cli.GetTx(subnetID)
	if err != nil {
		return nil, nil, err
	}

	tx := new(platformvm.Tx)
	if _, err = pCodecManager.Unmarshal(tb, tx); err != nil {
		return nil, nil, err
	}

	subnetTx, ok := tx.UnsignedTx.(*platformvm.UnsignedCreateSubnetTx)
	if !ok {
		return nil, nil, ErrWrongTxType
	}

	owner, ok := subnetTx.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return nil, nil, ErrUnknownOwners
	}

	kc := secp256k1fx.NewKeychain(k.Key())
	now := uint64(time.Now().Unix())
	indices, signers, ok := kc.Match(owner, now)
	if !ok {
		return nil, nil, ErrCantSign
	}
	return &secp256k1fx.Input{SigIndices: indices}, signers, nil
}
