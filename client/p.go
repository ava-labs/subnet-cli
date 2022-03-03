// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	ledger "github.com/ava-labs/avalanche-ledger-go"
	api_info "github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/status"
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
	Balance(addr string) (uint64, error)
	CreateSubnet(
		ctx context.Context,
		key key.Key,
		opts ...OpOption,
	) (subnetID ids.ID, took time.Duration, err error)
	AddValidator(
		ctx context.Context,
		l *ledger.Ledger,
		nodeID ids.ShortID,
		start time.Time,
		end time.Time,
		opts ...OpOption,
	) (took time.Duration, err error)
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
	GetValidator(rsubnetID ids.ID, nodeID ids.ShortID) (start time.Time, end time.Time, err error)
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

func ledgerAddress(l *ledger.Ledger) (string, ids.ShortID) {
	rawAddr, err := l.Address("fuji", 0, 0)
	if err != nil {
		panic(err)
	}
	_, pk, err := formatting.ParseBech32(rawAddr)
	if err != nil {
		panic(err)
	}
	addr, err := ids.ToShortID(pk)
	if err != nil {
		panic(err)
	}
	fullAddr, err := formatting.FormatAddress("P", "fuji", pk)
	if err != nil {
		panic(err)
	}
	return fullAddr, addr
}

// Sign this transaction with the provided ledger
func ledgerSignTx(tx *platformvm.Tx, inputs int, c codec.Manager, device *ledger.Ledger) error {
	unsignedBytes, err := c.Marshal(platformvm.CodecVersion, &tx.UnsignedTx)
	if err != nil {
		return fmt.Errorf("couldn't marshal UnsignedTx: %w", err)
	}

	// Attach credentials
	hash := hashing.ComputeHash256(unsignedBytes)
	cred := &secp256k1fx.Credential{
		Sigs: make([][crypto.SECP256K1RSigLen]byte, 1),
	}
	sig, err := device.SignHash(hash, [][]uint32{{0, 0}})
	if err != nil {
		return fmt.Errorf("problem generating credential: %w", err)
	}
	copy(cred.Sigs[0][:], sig[0])
	// CLEANUP
	for i := 0; i < inputs; i++ {
		tx.Creds = append(tx.Creds, cred) // Attach credential
	}

	signedBytes, err := c.Marshal(platformvm.CodecVersion, tx)
	if err != nil {
		return fmt.Errorf("couldn't marshal ProposalTx: %w", err)
	}
	tx.Initialize(unsignedBytes, signedBytes)
	return nil
}

func ledgerManagerSpends(addr ids.ShortID, outputs []*avax.UTXO, opts ...key.OpOption) (
	totalBalanceToSpend uint64,
	inputs []*avax.TransferableInput,
) {
	ret := &key.Op{}
	ret.ApplyOpts(opts)

	for _, out := range outputs {
		input, err := ledgerManagerSpend(addr, out, ret.Time)
		if err != nil {
			zap.L().Warn("cannot spend with current key", zap.Error(err))
			continue
		}
		totalBalanceToSpend += input.Amount()
		inputs = append(inputs, &avax.TransferableInput{
			UTXOID: out.UTXOID,
			Asset:  out.Asset,
			In:     input,
		})
		if ret.TargetAmount > 0 &&
			totalBalanceToSpend > ret.TargetAmount+ret.FeeDeduct {
			break
		}
	}
	avax.SortTransferableInputs(inputs)

	return totalBalanceToSpend, inputs
}

func ledgerManagerSpend(addr ids.ShortID, output *avax.UTXO, time uint64) (
	input avax.TransferableIn,
	err error,
) {
	// "time" is used to check whether the key owner
	// is still within the lock time (thus can't spend).
	inputf, err := ledgerSpend(addr, output.Out, time)
	if err != nil {
		return nil, err
	}
	var ok bool
	input, ok = inputf.(avax.TransferableIn)
	if !ok {
		return nil, errors.New("invalid type")
	}
	return input, nil
}

// Spend attempts to create an input
func ledgerSpend(addr ids.ShortID, out verify.Verifiable, time uint64) (verify.Verifiable, error) {
	switch out := out.(type) {
	case *secp256k1fx.MintOutput:
		if sigIndices, able := ledgerMatch(addr, &out.OutputOwners, time); able {
			return &secp256k1fx.Input{
				SigIndices: sigIndices,
			}, nil
		}
		return nil, errors.New("can't spend")
	case *secp256k1fx.TransferOutput:
		if sigIndices, able := ledgerMatch(addr, &out.OutputOwners, time); able {
			return &secp256k1fx.TransferInput{
				Amt: out.Amt,
				Input: secp256k1fx.Input{
					SigIndices: sigIndices,
				},
			}, nil
		}
		return nil, errors.New("can't spend")
	}
	return nil, fmt.Errorf("can't spend UTXO because it is unexpected type %T", out)
}

// Match attempts to match a list of addresses up to the provided threshold
func ledgerMatch(addr ids.ShortID, owners *secp256k1fx.OutputOwners, time uint64) ([]uint32, bool) {
	if time < owners.Locktime {
		return nil, false
	}
	sigs := make([]uint32, 0, owners.Threshold)
	for i := uint32(0); i < uint32(len(owners.Addrs)) && uint32(len(sigs)) < owners.Threshold; i++ {
		if owners.Addrs[i] == addr {
			fmt.Println("owner matches:", owners.Addrs[i])
			sigs = append(sigs, i)
		} else {
			a, err := formatting.FormatAddress("P", "fuji", owners.Addrs[i][:])
			if err != nil {
				panic(err)
			}
			fmt.Println("owner doesn't match:", owners.Addrs[i], a)
		}
	}
	return sigs, uint32(len(sigs)) == owners.Threshold
}

func (pc *p) Client() platformvm.Client            { return pc.cli }
func (pc *p) Checker() internal_platformvm.Checker { return pc.checker }

func (pc *p) Balance(addr string) (uint64, error) {
	// TODO: use ctx
	pb, err := pc.cli.GetBalance(context.Background(), []string{addr})
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
	panic("blah")
	// ret := &Op{}
	// ret.applyOpts(opts)

	// fi, err := pc.info.GetTxFee(ctx)
	// if err != nil {
	// 	return ids.Empty, 0, err
	// }
	// createSubnetTxFee := uint64(fi.CreateSubnetTxFee)

	// zap.L().Info("creating subnet",
	// 	zap.Bool("dryMode", ret.dryMode),
	// 	zap.String("assetId", pc.assetID.String()),
	// 	zap.Uint64("createSubnetTxFee", createSubnetTxFee),
	// )
	// ins, returnedOuts, _, signers, err := pc.stake(k, createSubnetTxFee)
	// if err != nil {
	// 	return ids.Empty, 0, err
	// }

	// utx := &platformvm.UnsignedCreateSubnetTx{
	// 	BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
	// 		NetworkID:    pc.networkID,
	// 		BlockchainID: pc.pChainID,
	// 		Ins:          ins,
	// 		Outs:         returnedOuts,
	// 	}},
	// 	Owner: &secp256k1fx.OutputOwners{
	// 		// [threshold] of [ownerAddrs] needed to manage this subnet
	// 		Threshold: 1,

	// 		// address to send change to, if there is any,
	// 		// control addresses for the new subnet
	// 		Addrs: []ids.ShortID{k.Key().PublicKey().Address()},
	// 	},
	// }
	// pTx := &platformvm.Tx{
	// 	UnsignedTx: utx,
	// }
	// if err := pTx.Sign(pCodecManager, signers); err != nil {
	// 	return ids.Empty, 0, err
	// }
	// if err := utx.SyntacticVerify(&snow.Context{
	// 	NetworkID: pc.networkID,
	// 	ChainID:   pc.pChainID,
	// }); err != nil {
	// 	return ids.Empty, 0, err
	// }

	// // subnet tx ID is the subnet ID based on ins/outs
	// subnetID = pTx.ID()
	// if ret.dryMode {
	// 	return subnetID, 0, nil
	// }

	// txID, err := pc.cli.IssueTx(ctx, pTx.Bytes())
	// if err != nil {
	// 	return subnetID, 0, fmt.Errorf("failed to issue tx: %w", err)
	// }
	// if txID != subnetID {
	// 	return subnetID, 0, ErrUnexpectedSubnetID
	// }

	// took, err = pc.checker.PollSubnet(ctx, txID)
	// return txID, took, err
}

func (pc *p) GetValidator(rsubnetID ids.ID, nodeID ids.ShortID) (start time.Time, end time.Time, err error) {
	// TODO: use ctx
	//
	// If no [rsubnetID] is provided, just use the PrimaryNetworkID value.
	subnetID := constants.PrimaryNetworkID
	if rsubnetID != ids.Empty {
		subnetID = rsubnetID
	}

	// Find validator data associated with [nodeID]
	vs, err := pc.Client().GetCurrentValidators(context.Background(), subnetID, []ids.ShortID{nodeID})
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	// If the validator is not found, it will return a string record indicating
	// that it was "unable to get mainnet validator record".
	if len(vs) < 1 {
		return time.Time{}, time.Time{}, ErrValidatorNotFound
	}
	var validator map[string]interface{}
	for _, v := range vs {
		va, ok := v.(map[string]interface{})
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: %T %+v", ErrInvalidValidatorData, v, v)
		}
		nodeIDs, ok := va["nodeID"].(string)
		if !ok {
			return time.Time{}, time.Time{}, ErrInvalidValidatorData
		}
		if nodeIDs == nodeID.PrefixedString(constants.NodeIDPrefix) {
			validator = va
			break
		}
	}
	if validator == nil {
		// This should never happen if the length of [vs] > 1, however,
		// we defend against it in case.
		return time.Time{}, time.Time{}, ErrValidatorNotFound
	}
	// Parse start/end time once the validator data is found (of format
	// `json.Uint64`)
	d, ok := validator["startTime"].(string)
	if !ok {
		return time.Time{}, time.Time{}, ErrInvalidValidatorData
	}
	dv, err := strconv.ParseInt(d, 10, 64)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start = time.Unix(dv, 0)
	d, ok = validator["endTime"].(string)
	if !ok {
		return time.Time{}, time.Time{}, ErrInvalidValidatorData
	}
	dv, err = strconv.ParseInt(d, 10, 64)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end = time.Unix(dv, 0)
	return start, end, nil
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
	panic("blah")
	// ret := &Op{}
	// ret.applyOpts(opts)

	// if subnetID == ids.Empty {
	// 	// same as "ErrNamedSubnetCantBePrimary"
	// 	// in case "subnetID == constants.PrimaryNetworkID"
	// 	return 0, ErrEmptyID
	// }
	// if nodeID == ids.ShortEmpty {
	// 	return 0, ErrEmptyID
	// }

	// _, _, err = pc.GetValidator(subnetID, nodeID)
	// if !errors.Is(err, ErrValidatorNotFound) {
	// 	return 0, ErrAlreadySubnetValidator
	// }

	// validateStart, validateEnd, err := pc.GetValidator(ids.ID{}, nodeID)
	// if errors.Is(err, ErrValidatorNotFound) {
	// 	return 0, ErrNotValidatingPrimaryNetwork
	// } else if err != nil {
	// 	return 0, fmt.Errorf("%w: unable to get primary network validator record", err)
	// }
	// // make sure the range is within staker validation start/end on the primary network
	// // TODO: official wallet client should define the error value for such case
	// // currently just returns "staking too short"
	// if start.Before(validateStart) {
	// 	return 0, fmt.Errorf("%w (validate start %v expected >%v)", ErrInvalidSubnetValidatePeriod, start, validateStart)
	// }
	// if end.After(validateEnd) {
	// 	return 0, fmt.Errorf("%w (validate end %v expected <%v)", ErrInvalidSubnetValidatePeriod, end, validateEnd)
	// }

	// fi, err := pc.info.GetTxFee(ctx)
	// if err != nil {
	// 	return 0, err
	// }
	// txFee := uint64(fi.TxFee)

	// zap.L().Info("adding subnet validator",
	// 	zap.String("subnetId", subnetID.String()),
	// 	zap.Uint64("txFee", txFee),
	// 	zap.Time("start", start),
	// 	zap.Time("end", end),
	// 	zap.Uint64("weight", weight),
	// )
	// ins, returnedOuts, _, signers, err := pc.stake(k, txFee)
	// if err != nil {
	// 	return 0, err
	// }
	// subnetAuth, subnetSigners, err := pc.authorize(k, subnetID)
	// if err != nil {
	// 	return 0, err
	// }
	// signers = append(signers, subnetSigners)

	// utx := &platformvm.UnsignedAddSubnetValidatorTx{
	// 	BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
	// 		NetworkID:    pc.networkID,
	// 		BlockchainID: pc.pChainID,
	// 		Ins:          ins,
	// 		Outs:         returnedOuts,
	// 	}},
	// 	Validator: platformvm.SubnetValidator{
	// 		Validator: platformvm.Validator{
	// 			NodeID: nodeID,
	// 			Start:  uint64(start.Unix()),
	// 			End:    uint64(end.Unix()),
	// 			Wght:   weight,
	// 		},
	// 		Subnet: subnetID,
	// 	},
	// 	SubnetAuth: subnetAuth,
	// }
	// pTx := &platformvm.Tx{
	// 	UnsignedTx: utx,
	// }
	// if err := pTx.Sign(pCodecManager, signers); err != nil {
	// 	return 0, err
	// }
	// if err := utx.SyntacticVerify(&snow.Context{
	// 	NetworkID: pc.networkID,
	// 	ChainID:   pc.pChainID,
	// }); err != nil {
	// 	return 0, err
	// }
	// txID, err := pc.cli.IssueTx(ctx, pTx.Bytes())
	// if err != nil {
	// 	return 0, fmt.Errorf("failed to issue tx: %w", err)
	// }

	// return pc.checker.PollTx(ctx, txID, status.Committed)
}

// ref. "platformvm.VM.newAddValidatorTx".
func (pc *p) AddValidator(
	ctx context.Context,
	l *ledger.Ledger,
	nodeID ids.ShortID,
	start time.Time,
	end time.Time,
	opts ...OpOption,
) (took time.Duration, err error) {
	fullAddr, addr := ledgerAddress(l)
	ret := &Op{}
	ret.applyOpts(opts)

	if nodeID == ids.ShortEmpty {
		return 0, ErrEmptyID
	}

	_, _, err = pc.GetValidator(ids.ID{}, nodeID)
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
		ret.rewardAddr = addr
		zap.L().Warn("reward address not set, default to self",
			zap.String("rewardAddress", ret.rewardAddr.String()),
		)
	}
	if ret.changeAddr == ids.ShortEmpty {
		ret.changeAddr = addr
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

	ins, returnedOuts, stakedOuts, err := pc.stake(
		fullAddr,
		addr,
		addStakerTxFee,
		WithStakeAmount(ret.stakeAmt),
		WithRewardAddress(ret.rewardAddr),
		WithRewardShares(ret.rewardShares),
		WithChangeAddress(ret.changeAddr),
	)
	if err != nil {
		return 0, err
	}

	utx := &platformvm.UnsignedAddValidatorTx{
		BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    pc.networkID,
			BlockchainID: pc.pChainID,
			Ins:          ins,
			Outs:         returnedOuts,
		}},
		Validator: platformvm.Validator{
			NodeID: nodeID,
			Start:  uint64(start.Unix()),
			End:    uint64(end.Unix()),
			Wght:   ret.stakeAmt,
		},
		Stake: stakedOuts,
		RewardsOwner: &secp256k1fx.OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs:     []ids.ShortID{ret.rewardAddr},
		},
		Shares: ret.rewardShares,
	}
	pTx := &platformvm.Tx{
		UnsignedTx: utx,
	}
	if err := ledgerSignTx(pTx, len(ins), pCodecManager, l); err != nil {
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

	return pc.checker.PollTx(ctx, txID, status.Committed)
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
	panic("blah")
	// ret := &Op{}
	// ret.applyOpts(opts)

	// if subnetID == ids.Empty {
	// 	return ids.Empty, 0, ErrEmptyID
	// }
	// if vmID == ids.Empty {
	// 	return ids.Empty, 0, ErrEmptyID
	// }

	// fi, err := pc.info.GetTxFee(ctx)
	// if err != nil {
	// 	return ids.Empty, 0, err
	// }
	// createBlkChainTxFee := uint64(fi.CreateBlockchainTxFee)

	// now := time.Now()
	// zap.L().Info("creating blockchain",
	// 	zap.String("subnetId", subnetID.String()),
	// 	zap.String("chainName", chainName),
	// 	zap.String("vmId", vmID.String()),
	// 	zap.Uint64("createBlockchainTxFee", createBlkChainTxFee),
	// )
	// panic("not implemented")
	// ins, returnedOuts, _, signers, err := pc.stake(k, createBlkChainTxFee)
	// if err != nil {
	// 	return ids.Empty, 0, err
	// }
	// subnetAuth, subnetSigners, err := pc.authorize(k, subnetID)
	// if err != nil {
	// 	return ids.Empty, 0, err
	// }
	// signers = append(signers, subnetSigners)

	// utx := &platformvm.UnsignedCreateChainTx{
	// 	BaseTx: platformvm.BaseTx{BaseTx: avax.BaseTx{
	// 		NetworkID:    pc.networkID,
	// 		BlockchainID: pc.pChainID,
	// 		Ins:          ins,
	// 		Outs:         returnedOuts,
	// 	}},
	// 	SubnetID:    subnetID,
	// 	ChainName:   chainName,
	// 	VMID:        vmID,
	// 	FxIDs:       nil,
	// 	GenesisData: vmGenesis,
	// 	SubnetAuth:  subnetAuth,
	// }
	// pTx := &platformvm.Tx{
	// 	UnsignedTx: utx,
	// }
	// if err := pTx.Sign(pCodecManager, signers); err != nil {
	// 	return ids.Empty, 0, err
	// }
	// if err := utx.SyntacticVerify(&snow.Context{
	// 	NetworkID: pc.networkID,
	// 	ChainID:   pc.pChainID,
	// }); err != nil {
	// 	return ids.Empty, 0, err
	// }
	// blkChainID, err = pc.cli.IssueTx(ctx, pTx.Bytes())
	// if err != nil {
	// 	return ids.Empty, 0, fmt.Errorf("failed to issue tx: %w", err)
	// }

	// took = time.Since(now)
	// if ret.poll {
	// 	var bTook time.Duration
	// 	bTook, err = pc.checker.PollBlockchain(
	// 		ctx,
	// 		internal_platformvm.WithSubnetID(subnetID),
	// 		internal_platformvm.WithBlockchainID(blkChainID),
	// 		internal_platformvm.WithBlockchainStatus(status.Validating),
	// 		internal_platformvm.WithCheckBlockchainBootstrapped(pc.info),
	// 	)
	// 	took += bTook
	// }
	// return blkChainID, took, err
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
func (pc *p) stake(fullAddr string, addr ids.ShortID, fee uint64, opts ...OpOption) (
	ins []*avax.TransferableInput,
	returnedOuts []*avax.TransferableOutput,
	stakedOuts []*avax.TransferableOutput,
	err error,
) {
	ret := &Op{}
	ret.applyOpts(opts)
	if ret.rewardAddr == ids.ShortEmpty {
		ret.rewardAddr = addr
	}
	if ret.changeAddr == ids.ShortEmpty {
		ret.changeAddr = addr
	}

	ubs, _, err := pc.cli.GetAtomicUTXOs(context.Background(), []string{fullAddr}, "", 100, "", "")
	if err != nil {
		return nil, nil, nil, err
	}

	now := uint64(time.Now().Unix())

	ins = make([]*avax.TransferableInput, 0)
	returnedOuts = make([]*avax.TransferableOutput, 0)
	stakedOuts = make([]*avax.TransferableOutput, 0)

	utxos := make([]*avax.UTXO, len(ubs))
	for i, ub := range ubs {
		utxos[i], err = internal_avax.ParseUTXO(ub, pCodecManager)
		if err != nil {
			return nil, nil, nil, err
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

		out, ok := utxo.Out.(*platformvm.StakeableLockOut)
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

		_, inputs := ledgerManagerSpends(addr, []*avax.UTXO{utxo}, key.WithTime(now))
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
			Out: &platformvm.StakeableLockOut{
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
		inner, ok := out.(*platformvm.StakeableLockOut)
		if ok {
			if inner.Locktime > now {
				// output currently locked, can't be burned
				// skip for next UTXO
				continue
			}
			utxo.Out = inner.TransferableOut
		}
		_, inputs := ledgerManagerSpends(addr, []*avax.UTXO{utxo}, key.WithTime(now))
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
	}

	if amountStaked > 0 && amountStaked < ret.stakeAmt {
		return nil, nil, nil, ErrInsufficientBalanceForStakeAmount
	}
	if amountBurned > 0 && amountBurned < fee {
		return nil, nil, nil, ErrInsufficientBalanceForGasFee
	}

	avax.SortTransferableInputs(ins)                          // sort inputs and keys
	avax.SortTransferableOutputs(returnedOuts, pCodecManager) // sort outputs
	avax.SortTransferableOutputs(stakedOuts, pCodecManager)   // sort outputs

	return ins, returnedOuts, stakedOuts, nil
}

// ref. "platformvm.VM.authorize".
func (pc *p) authorize(k key.Key, subnetID ids.ID) (
	auth verify.Verifiable, // input that names owners
	signers []*crypto.PrivateKeySECP256K1R, // keys that prove ownership
	err error,
) {
	tb, err := pc.cli.GetTx(context.Background(), subnetID)
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
