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
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/stakeable"
	pstatus "github.com/ava-labs/avalanchego/vms/platformvm/status"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/validator"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	internal_avax "github.com/ava-labs/subnet-cli/internal/avax"
	"github.com/ava-labs/subnet-cli/internal/key"
	internal_platformvm "github.com/ava-labs/subnet-cli/internal/platformvm"
	"go.uber.org/zap"
)

var (
	ErrInsufficientBalanceForGasFee      = errors.New("insufficient balance for gas")
	ErrInsufficientBalanceForStakeAmount = errors.New("insufficient balance for stake amount")
	ErrUnexpectedSubnetID                = errors.New("unexpected subnet ID")

	ErrEmptyValidator              = errors.New("empty validator set")
	ErrAlreadyValidator            = errors.New("already validator")
	ErrAlreadySubnetValidator      = errors.New("already subnet validator")
	ErrNotValidatingPrimaryNetwork = errors.New("validator not validating the primary network")
	ErrInvalidSubnetValidatePeriod = errors.New("invalid subnet validate period")
	ErrInvalidValidatorData        = errors.New("invalid validator data")
	ErrValidatorNotFound           = errors.New("validator not found")

	// ref. "vms.platformvm".
	ErrWrongTxType   = errors.New("wrong transaction type")
	ErrUnknownOwners = errors.New("unknown owners")
	ErrCantSign      = errors.New("can't sign")
)

type P interface {
	Client() platformvm.Client
	Checker() internal_platformvm.Checker
	Balance(ctx context.Context, key key.Key) (uint64, error)
	CreateSubnet(
		ctx context.Context,
		key key.Key,
		opts ...OpOption,
	) (subnetID ids.ID, took time.Duration, err error)
	AddValidator(
		ctx context.Context,
		k key.Key,
		nodeID ids.NodeID,
		start time.Time,
		end time.Time,
		opts ...OpOption,
	) (took time.Duration, err error)
	AddSubnetValidator(
		ctx context.Context,
		k key.Key,
		subnetID ids.ID,
		nodeID ids.NodeID,
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
	GetValidator(
		ctx context.Context,
		rsubnetID ids.ID,
		nodeID ids.NodeID,
	) (start time.Time, end time.Time, err error)
}

type p struct {
	cfg         Config
	networkName string
	networkID   uint32
	assetID     ids.ID
	pChainID    ids.ID

	cli     platformvm.Client
	info    api_info.Client
	checker internal_platformvm.Checker
}

func (pc *p) Client() platformvm.Client            { return pc.cli }
func (pc *p) Checker() internal_platformvm.Checker { return pc.checker }

func (pc *p) Balance(ctx context.Context, key key.Key) (uint64, error) {
	pb, err := pc.cli.GetBalance(ctx, key.Addresses())
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

	fi, err := pc.info.GetTxFee(ctx)
	if err != nil {
		return ids.Empty, 0, err
	}
	createSubnetTxFee := uint64(fi.CreateSubnetTxFee)

	zap.L().Info("creating subnet",
		zap.Bool("dryMode", ret.dryMode),
		zap.String("assetId", pc.assetID.String()),
		zap.Uint64("createSubnetTxFee", createSubnetTxFee),
	)
	ins, returnedOuts, _, signers, err := pc.stake(ctx, k, createSubnetTxFee)
	if err != nil {
		return ids.Empty, 0, err
	}

	utx := &txs.CreateSubnetTx{
		BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         returnedOuts,
		}},
		Owner: &secp256k1fx.OutputOwners{
			// [threshold] of [ownerAddrs] needed to manage this subnet
			Threshold: 1,

			// address to send change to, if there is any,
			// control addresses for the new subnet
			Addrs: []ids.ShortID{k.Addresses()[0]},
		},
	}
	pTx := &txs.Tx{
		Unsigned: utx,
	}
	if err := k.Sign(pTx, signers); err != nil {
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

	txID, err := pc.cli.IssueTx(ctx, pTx.Bytes())
	if err != nil {
		return subnetID, 0, fmt.Errorf("failed to issue tx: %w", err)
	}
	if txID != subnetID {
		return subnetID, 0, ErrUnexpectedSubnetID
	}

	took, err = pc.checker.PollSubnet(ctx, txID)
	return txID, took, err
}

func (pc *p) GetValidator(ctx context.Context, rsubnetID ids.ID, nodeID ids.NodeID) (start time.Time, end time.Time, err error) {
	// If no [rsubnetID] is provided, just use the PrimaryNetworkID value.
	subnetID := constants.PrimaryNetworkID
	if rsubnetID != ids.Empty {
		subnetID = rsubnetID
	}

	// Find validator data associated with [nodeID]
	vs, err := pc.Client().GetCurrentValidators(ctx, subnetID, []ids.NodeID{nodeID})
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	// If the validator is not found, it will return a string record indicating
	// that it was "unable to get mainnet validator record".
	if len(vs) < 1 {
		return time.Time{}, time.Time{}, ErrValidatorNotFound
	}
	for _, v := range vs {
		if v.NodeID == nodeID {
			return time.Unix(int64(v.StartTime), 0), time.Unix(int64(v.EndTime), 0), nil
		}
	}
	// This should never happen if the length of [vs] > 1, however,
	// we defend against it in case.
	return time.Time{}, time.Time{}, ErrValidatorNotFound
}

// ref. "platformvm.VM.newAddSubnetValidatorTx".
func (pc *p) AddSubnetValidator(
	ctx context.Context,
	k key.Key,
	subnetID ids.ID,
	nodeID ids.NodeID,
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
	if nodeID == ids.EmptyNodeID {
		return 0, ErrEmptyID
	}

	_, _, err = pc.GetValidator(ctx, subnetID, nodeID)
	if !errors.Is(err, ErrValidatorNotFound) {
		return 0, ErrAlreadySubnetValidator
	}

	validateStart, validateEnd, err := pc.GetValidator(ctx, ids.ID{}, nodeID)
	if errors.Is(err, ErrValidatorNotFound) {
		return 0, ErrNotValidatingPrimaryNetwork
	} else if err != nil {
		return 0, fmt.Errorf("%w: unable to get primary network validator record", err)
	}
	// make sure the range is within staker validation start/end on the primary network
	// TODO: official wallet client should define the error value for such case
	// currently just returns "staking too short"
	if start.Before(validateStart) {
		return 0, fmt.Errorf("%w (validate start %v expected >%v)", ErrInvalidSubnetValidatePeriod, start, validateStart)
	}
	if end.After(validateEnd) {
		return 0, fmt.Errorf("%w (validate end %v expected <%v)", ErrInvalidSubnetValidatePeriod, end, validateEnd)
	}

	fi, err := pc.info.GetTxFee(ctx)
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
	ins, returnedOuts, _, signers, err := pc.stake(ctx, k, txFee)
	if err != nil {
		return 0, err
	}
	subnetAuth, subnetSigners, err := pc.authorize(ctx, k, subnetID)
	if err != nil {
		return 0, err
	}
	signers = append(signers, subnetSigners)

	utx := &txs.AddSubnetValidatorTx{
		BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         returnedOuts,
		}},
		Validator: validator.SubnetValidator{
			Validator: validator.Validator{
				NodeID: nodeID,
				Start:  uint64(start.Unix()),
				End:    uint64(end.Unix()),
				Wght:   weight,
			},
			Subnet: subnetID,
		},
		SubnetAuth: subnetAuth,
	}
	pTx := &txs.Tx{
		Unsigned: utx,
	}
	if err := k.Sign(pTx, signers); err != nil {
		return 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return 0, err
	}
	txID, err := pc.cli.IssueTx(ctx, pTx.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to issue tx: %w", err)
	}

	return pc.checker.PollTx(ctx, txID, pstatus.Committed)
}

// ref. "platformvm.VM.newAddValidatorTx".
func (pc *p) AddValidator(
	ctx context.Context,
	k key.Key,
	nodeID ids.NodeID,
	start time.Time,
	end time.Time,
	opts ...OpOption,
) (took time.Duration, err error) {
	ret := &Op{}
	ret.applyOpts(opts)

	if nodeID == ids.EmptyNodeID {
		return 0, ErrEmptyID
	}

	_, _, err = pc.GetValidator(ctx, ids.ID{}, nodeID)
	if err == nil {
		return 0, ErrAlreadyValidator
	} else if !errors.Is(err, ErrValidatorNotFound) {
		return 0, err
	}

	// ref. https://docs.avax.network/learn/platform-overview/staking/#staking-parameters-on-avalanche
	// ref. https://docs.avax.network/learn/platform-overview/staking/#validating-in-fuji
	if ret.stakeAmt == 0 {
		switch pc.networkName {
		case constants.MainnetName:
			ret.stakeAmt = 2000 * units.Avax
		case constants.LocalName,
			constants.FujiName:
			ret.stakeAmt = 1 * units.Avax
		}
		zap.L().Info("stake amount not set, default to network setting",
			zap.String("networkName", pc.networkName),
			zap.Uint64("stakeAmount", ret.stakeAmt),
		)
	}
	if ret.rewardAddr == ids.ShortEmpty {
		ret.rewardAddr = k.Addresses()[0]
		zap.L().Warn("reward address not set, default to self",
			zap.String("rewardAddress", ret.rewardAddr.String()),
		)
	}
	if ret.changeAddr == ids.ShortEmpty {
		ret.changeAddr = k.Addresses()[0]
		zap.L().Warn("change address not set",
			zap.String("changeAddress", ret.changeAddr.String()),
		)
	}

	zap.L().Info("adding validator",
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Uint64("stakeAmount", ret.stakeAmt),
		zap.String("rewardAddress", ret.rewardAddr.String()),
		zap.String("changeAddress", ret.changeAddr.String()),
	)

	// ref. https://docs.avax.network/learn/platform-overview/transaction-fees/#fee-schedule
	addStakerTxFee := uint64(0)

	ins, returnedOuts, stakedOuts, signers, err := pc.stake(
		ctx,
		k,
		addStakerTxFee,
		WithStakeAmount(ret.stakeAmt),
		WithRewardAddress(ret.rewardAddr),
		WithRewardShares(ret.rewardShares),
		WithChangeAddress(ret.changeAddr),
	)
	if err != nil {
		return 0, err
	}

	utx := &txs.AddValidatorTx{
		BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         returnedOuts,
		}},
		Validator: validator.Validator{
			NodeID: nodeID,
			Start:  uint64(start.Unix()),
			End:    uint64(end.Unix()),
			Wght:   ret.stakeAmt,
		},
		StakeOuts: stakedOuts,
		RewardsOwner: &secp256k1fx.OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs:     []ids.ShortID{ret.rewardAddr},
		},
		DelegationShares: ret.rewardShares,
	}
	pTx := &txs.Tx{
		Unsigned: utx,
	}
	if err := k.Sign(pTx, signers); err != nil {
		return 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return 0, err
	}
	txID, err := pc.cli.IssueTx(ctx, pTx.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to issue tx: %w", err)
	}

	return pc.checker.PollTx(ctx, txID, pstatus.Committed)
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

	fi, err := pc.info.GetTxFee(ctx)
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
	ins, returnedOuts, _, signers, err := pc.stake(ctx, k, createBlkChainTxFee)
	if err != nil {
		return ids.Empty, 0, err
	}
	subnetAuth, subnetSigners, err := pc.authorize(ctx, k, subnetID)
	if err != nil {
		return ids.Empty, 0, err
	}
	signers = append(signers, subnetSigners)

	utx := &txs.CreateChainTx{
		BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         returnedOuts,
		}},
		SubnetID:    subnetID,
		ChainName:   chainName,
		VMID:        vmID,
		FxIDs:       nil,
		GenesisData: vmGenesis,
		SubnetAuth:  subnetAuth,
	}
	pTx := &txs.Tx{
		Unsigned: utx,
	}
	if err := k.Sign(pTx, signers); err != nil {
		return ids.Empty, 0, err
	}
	if err := utx.SyntacticVerify(&snow.Context{
		NetworkID: pc.networkID,
		ChainID:   pc.pChainID,
	}); err != nil {
		return ids.Empty, 0, err
	}
	blkChainID, err = pc.cli.IssueTx(ctx, pTx.Bytes())
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
			internal_platformvm.WithBlockchainStatus(pstatus.Validating),
			internal_platformvm.WithCheckBlockchainBootstrapped(pc.info),
		)
		took += bTook
	}
	return blkChainID, took, err
}

type Op struct {
	stakeAmt     uint64
	rewardShares uint32
	rewardAddr   ids.ShortID
	changeAddr   ids.ShortID

	dryMode bool
	poll    bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithStakeAmount(v uint64) OpOption {
	return func(op *Op) {
		op.stakeAmt = v
	}
}

func WithRewardShares(v uint32) OpOption {
	return func(op *Op) {
		op.rewardShares = v
	}
}

func WithRewardAddress(v ids.ShortID) OpOption {
	return func(op *Op) {
		op.rewardAddr = v
	}
}

func WithChangeAddress(v ids.ShortID) OpOption {
	return func(op *Op) {
		op.changeAddr = v
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
func (pc *p) stake(ctx context.Context, k key.Key, fee uint64, opts ...OpOption) (
	ins []*avax.TransferableInput,
	returnedOuts []*avax.TransferableOutput,
	stakedOuts []*avax.TransferableOutput,
	signers [][]ids.ShortID,
	err error,
) {
	ret := &Op{}
	ret.applyOpts(opts)
	if ret.rewardAddr == ids.ShortEmpty {
		ret.rewardAddr = k.Addresses()[0]
	}
	if ret.changeAddr == ids.ShortEmpty {
		ret.changeAddr = k.Addresses()[0]
	}

	ubs, _, _, err := pc.cli.GetAtomicUTXOs(ctx, k.Addresses(), "", 100, ids.ShortEmpty, ids.Empty)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	now := uint64(time.Now().Unix())

	ins = make([]*avax.TransferableInput, 0)
	returnedOuts = make([]*avax.TransferableOutput, 0)
	stakedOuts = make([]*avax.TransferableOutput, 0)

	utxos := make([]*avax.UTXO, len(ubs))
	for i, ub := range ubs {
		utxos[i], err = internal_avax.ParseUTXO(ub, txs.Codec)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	// amount of AVAX that has been staked
	amountStaked := uint64(0)
	for _, utxo := range utxos {
		// have staked more AVAX then we need to
		// no need to consume more AVAX
		if amountStaked >= ret.stakeAmt {
			break
		}
		// assume "AssetID" is set to "AVAX" asset ID
		if utxo.AssetID() != pc.assetID {
			continue
		}

		out, ok := utxo.Out.(*stakeable.LockOut)
		if !ok {
			// This output isn't locked, so it will be handled during the next
			// iteration of the UTXO set
			continue
		}
		if out.Locktime <= now {
			// This output is no longer locked, so it will be handled during the
			// next iteration of the UTXO set
			continue
		}

		inner, ok := out.TransferableOut.(*secp256k1fx.TransferOutput)
		if !ok {
			// We only know how to clone secp256k1 outputs for now
			continue
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
			ret.stakeAmt-amountStaked, // Amount we still need to stake
			remainingValue,            // Amount available to stake
		)
		amountStaked += amountToStake
		remainingValue -= amountToStake

		// Add the output to the staked outputs
		stakedOuts = append(stakedOuts, &avax.TransferableOutput{
			Asset: avax.Asset{ID: pc.assetID},
			Out: &stakeable.LockOut{
				Locktime: out.Locktime,
				TransferableOut: &secp256k1fx.TransferOutput{
					Amt:          amountToStake,
					OutputOwners: inner.OutputOwners,
				},
			},
		})

		if remainingValue > 0 {
			// input had extra value, so some of it must be returned
			returnedOuts = append(returnedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: pc.assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: remainingValue,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,

						// address to send change to, if there is any
						Addrs: []ids.ShortID{ret.changeAddr},
					},
				},
			})
		}

		// add the input to the consumed inputs
		ins = append(ins, in)
		signers = append(signers, inputSigners...)
	}

	// amount of AVAX that has been burned
	amountBurned := uint64(0)
	for _, utxo := range utxos {
		// have staked more AVAX then we need to
		// have burned more AVAX then we need to
		// no need to consume more AVAX
		if amountStaked >= ret.stakeAmt && amountBurned >= fee {
			break
		}
		// assume "AssetID" is set to "AVAX" asset ID
		if utxo.AssetID() != pc.assetID {
			continue
		}

		out := utxo.Out
		inner, ok := out.(*stakeable.LockOut)
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

		// stake any value that should be staked
		amountToStake := math.Min64(
			ret.stakeAmt-amountStaked, // Amount we still need to stake
			remainingValue,            // Amount available to stake
		)
		amountStaked += amountToStake
		remainingValue -= amountToStake

		if amountToStake > 0 {
			// Some of this input was put for staking
			stakedOuts = append(stakedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: pc.assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: amountToStake,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{ret.changeAddr},
					},
				},
			})
		}

		if remainingValue > 0 {
			// input had extra value, so some of it must be returned
			returnedOuts = append(returnedOuts, &avax.TransferableOutput{
				Asset: avax.Asset{ID: pc.assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: remainingValue,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,

						// address to send change to, if there is any
						Addrs: []ids.ShortID{ret.changeAddr},
					},
				},
			})
		}

		// add the input to the consumed inputs
		ins = append(ins, in)
		signers = append(signers, inputSigners...)
	}

	if amountStaked > 0 && amountStaked < ret.stakeAmt {
		return nil, nil, nil, nil, ErrInsufficientBalanceForStakeAmount
	}
	if amountBurned > 0 && amountBurned < fee {
		return nil, nil, nil, nil, ErrInsufficientBalanceForGasFee
	}

	key.SortTransferableInputsWithSigners(ins, signers)   // sort inputs
	avax.SortTransferableOutputs(returnedOuts, txs.Codec) // sort outputs
	avax.SortTransferableOutputs(stakedOuts, txs.Codec)   // sort outputs

	return ins, returnedOuts, stakedOuts, signers, nil
}

// ref. "platformvm.VM.authorize".
func (pc *p) authorize(ctx context.Context, k key.Key, subnetID ids.ID) (
	auth verify.Verifiable, // input that names owners
	signers []ids.ShortID,
	err error,
) {
	tb, err := pc.cli.GetTx(ctx, subnetID)
	if err != nil {
		return nil, nil, err
	}

	tx := new(txs.Tx)
	if _, err = txs.Codec.Unmarshal(tb, tx); err != nil {
		return nil, nil, err
	}

	subnetTx, ok := tx.Unsigned.(*txs.CreateSubnetTx)
	if !ok {
		return nil, nil, ErrWrongTxType
	}

	owner, ok := subnetTx.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return nil, nil, ErrUnknownOwners
	}
	now := uint64(time.Now().Unix())
	indices, signers, ok := k.Match(owner, now)
	if !ok {
		return nil, nil, ErrCantSign
	}
	return &secp256k1fx.Input{SigIndices: indices}, signers, nil
}
