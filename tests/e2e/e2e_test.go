// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// e2e implements the e2e tests.
package e2e_test

import (
	"context"
	"flag"
	"syscall"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/subnet-cli/internal/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	"github.com/ava-labs/subnet-cli/tests"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/zap"
)

func TestE2e(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "subnet-cli e2e test suites")
}

var (
	clusterInfoPath string
	logLevel        string
	shutdown        bool
)

func init() {
	flag.StringVar(
		&clusterInfoPath,
		"cluster-info-path",
		"",
		"cluster info YAML file path (as defined in 'tests/cluster_info.go')",
	)
	flag.StringVar(
		&logLevel,
		"log-level",
		logutil.DefaultLogLevel,
		"log level",
	)
	flag.BoolVar(
		&shutdown,
		"shutdown",
		false,
		"'true' to send SIGINT to the local cluster for shutdown",
	)
}

var (
	clusterInfo tests.ClusterInfo
	cli         client.Client
	k           key.Key
)

var _ = ginkgo.BeforeSuite(func() {
	var err error
	clusterInfo, err = tests.LoadClusterInfo(clusterInfoPath)
	gomega.Ω(err).Should(gomega.BeNil())

	lcfg := logutil.GetDefaultZapLoggerConfig()
	lcfg.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(logLevel))
	logger, err := lcfg.Build()
	gomega.Ω(err).Should(gomega.BeNil())
	_ = zap.ReplaceGlobals(logger)

	cli, err = client.New(client.Config{
		URI:            clusterInfo.URIs[0],
		PollInterval:   time.Second,
		RequestTimeout: time.Minute,
	})
	gomega.Ω(err).Should(gomega.BeNil())

	k, err = key.New(9999999, "test", key.WithPrivateKeyEncoded(key.EwoqPrivateKey))
	gomega.Ω(err).Should(gomega.BeNil())
})

var _ = ginkgo.AfterSuite(func() {
	if !shutdown {
		color.Outf("{{red}}skipping shutdown for PID %d{{/}}\n", clusterInfo.PID)
		return
	}
	color.Outf("{{red}}shutting down local cluster on PID %d{{/}}\n", clusterInfo.PID)
	serr := syscall.Kill(clusterInfo.PID, syscall.SIGTERM)
	color.Outf("{{red}}terminated local cluster on PID %d{{/}} (error %v)\n", clusterInfo.PID, serr)
})

var subnetID = ids.Empty

var _ = ginkgo.Describe("[CreateSubnet/CreateBlockchain]", func() {
	ginkgo.It("can issue CreateSubnetTx", func() {
		balance, err := cli.P().Balance(k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee()
		gomega.Ω(err).Should(gomega.BeNil())
		subnetTxFee := uint64(feeInfo.CreateSubnetTxFee)
		expectedBalance := balance - subnetTxFee

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		subnet1, _, err := cli.P().CreateSubnet(ctx, k, client.WithDryMode(true))
		cancel()
		gomega.Ω(err).Should(gomega.BeNil())

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		subnet2, _, err := cli.P().CreateSubnet(ctx, k, client.WithDryMode(false))
		cancel()
		gomega.Ω(err).Should(gomega.BeNil())

		ginkgo.By("returns an identical subnet ID with dry mode", func() {
			gomega.Ω(subnet1).Should(gomega.Equal(subnet2))
		})
		subnetID = subnet1

		ginkgo.By("returns a tx-fee deducted balance", func() {
			curBal, err := cli.P().Balance(k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})

	ginkgo.It("can issue AddSubnetValidatorTx", func() {
		balance, err := cli.P().Balance(k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee()
		gomega.Ω(err).Should(gomega.BeNil())
		txFee := uint64(feeInfo.TxFee)
		expectedBalance := balance - txFee

		nodeIDs, err := cli.Info().Client().GetNodeID()
		gomega.Ω(err).Should(gomega.BeNil())
		nodeID, err := ids.ShortFromPrefixedString(nodeIDs, constants.NodeIDPrefix)
		gomega.Ω(err).Should(gomega.BeNil())

		ginkgo.By("fails when subnet ID is empty", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				ids.Empty,
				nodeID,
				time.Now(),
				time.Now(),
				1000,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrEmptyID.Error()))
		})

		ginkgo.By("fails when node ID is empty", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				subnetID,
				ids.ShortEmpty,
				time.Now(),
				time.Now(),
				1000,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrEmptyID.Error()))
		})

		ginkgo.By("fails when validate start/end times are invalid", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				subnetID,
				nodeID,
				time.Now(),
				time.Now().Add(5*time.Second),
				1000,
			)
			cancel()
			// e.g., "failed to issue tx: couldn't issue tx: staking period is too short"
			gomega.Ω(err.Error()).Should(gomega.ContainSubstring("staking period is too short"))
		})

		ginkgo.By("successfully adds the subnet as a validator", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				subnetID,
				nodeID,
				time.Now().Add(30*time.Second),
				time.Now().Add(2*24*time.Hour),
				1000,
			)
			cancel()
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("returns a tx-fee deducted balance", func() {
			curBal, err := cli.P().Balance(k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})

	ginkgo.It("can issue AddSubnetValidatorTx (new validator)", func() {
		balance, err := cli.P().Balance(k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee()
		gomega.Ω(err).Should(gomega.BeNil())
		txFee := uint64(feeInfo.TxFee)
		expectedBalance := balance - txFee

		nodeID := ids.GenerateTestShortID()

		ginkgo.By("successfully adds the subnet as a validator", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				subnetID,
				nodeID,
				time.Now().Add(30*time.Second),
				time.Now().Add(2*24*time.Hour),
				1000,
			)
			cancel()
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("returns a tx-fee deducted balance", func() {
			curBal, err := cli.P().Balance(k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})

	ginkgo.It("can issue CreateBlockchain", func() {
		ginkgo.By("fails when subnet ID is empty", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, _, err := cli.P().CreateBlockchain(
				ctx,
				k,
				ids.Empty,
				"",
				ids.Empty,
				nil,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrEmptyID.Error()))
		})

		ginkgo.By("fails when vm ID is empty", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, _, err := cli.P().CreateBlockchain(
				ctx,
				k,
				subnetID,
				"",
				ids.Empty,
				nil,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrEmptyID.Error()))
		})

		ginkgo.Skip("TODO: once we have a testable custom VM in public")

		balance, err := cli.P().Balance(k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee()
		gomega.Ω(err).Should(gomega.BeNil())
		blkChainFee := uint64(feeInfo.CreateBlockchainTxFee)
		expectedBalance := balance - blkChainFee

		ginkgo.By("returns a tx-fee deducted balance", func() {
			curBal, err := cli.P().Balance(k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})
})
