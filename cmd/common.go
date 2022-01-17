package cmd

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"go.uber.org/zap"

	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
)

type Info struct {
	balance uint64
	feeData *info.GetTxFeeResponse
	txFee   uint64

	key key.Key

	networkName string

	pollInterval   time.Duration
	requestTimeout time.Duration

	subnetIDType string
	subnetID     ids.ID

	blockchainID  ids.ID
	vmName        string
	vmID          ids.ID
	vmGenesisPath string

	validateStart  time.Time
	validateEnd    time.Time
	validateWeight uint64
}

func InitClient() (client.Client, *Info, error) {
	cli, err := client.New(client.Config{
		URI:            uri,
		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return nil, nil, err
	}
	k, err := key.Load(cli.NetworkID(), privKeyPath)
	if err != nil {
		return nil, nil, err
	}

	balance, err := cli.P().Balance(k)
	if err != nil {
		return nil, nil, err
	}
	txFee, err := cli.Info().Client().GetTxFee()
	if err != nil {
		return nil, nil, err
	}
	networkName, err := cli.Info().Client().GetNetworkName()
	if err != nil {
		return nil, nil, err
	}

	info := &Info{
		balance:     balance,
		feeData:     txFee,
		networkName: networkName,
	}

	return cli, info, nil
}

func CreateLogger() error {
	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	if err != nil {
		return err
	}
	_ = zap.ReplaceGlobals(logger)
	return nil
}

func (i *Info) CheckBalance() error {
	if i.balance < uint64(i.txFee) {
		return fmt.Errorf("insuffient fee on %s (expected=%d, have=%d)", i.key.P(), i.txFee, i.balance)
	}
	return nil
}
