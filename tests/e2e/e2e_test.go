// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// e2e implements the e2e tests.
package e2e_test

import (
	"context"
	"flag"
	"testing"
	"time"

	runner_client "github.com/ava-labs/avalanche-network-runner/client"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/subnet-cli/client"
	"github.com/ava-labs/subnet-cli/internal/key"
	"github.com/ava-labs/subnet-cli/pkg/color"
	"github.com/ava-labs/subnet-cli/pkg/logutil"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestE2e(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "subnet-cli e2e test suites")
}

var (
	logLevel      string
	gRPCEp        string
	gRPCGatewayEp string
	execPath      string
)

func init() {
	flag.StringVar(
		&logLevel,
		"log-level",
		logutil.DefaultLogLevel,
		"log level",
	)
	flag.StringVar(
		&gRPCEp,
		"grpc-endpoint",
		"0.0.0.0:8080",
		"gRPC server endpoint",
	)
	flag.StringVar(
		&gRPCGatewayEp,
		"grpc-gateway-endpoint",
		"0.0.0.0:8081",
		"gRPC gateway endpoint",
	)
	flag.StringVar(
		&execPath,
		"avalanchego-path",
		"",
		"avalanchego executable path",
	)
}

var (
	runnerClient runner_client.Client
	cli          client.Client
	k            key.Key
)

var _ = ginkgo.BeforeSuite(func() {
	var err error
	runnerClient, err = runner_client.New(runner_client.Config{
		Endpoint:    gRPCEp,
		DialTimeout: 10 * time.Second,
	}, logging.NoLog{})
	gomega.Ω(err).Should(gomega.BeNil())

	// TODO: pass subnet whitelisting
	color.Outf("{{green}}starting:{{/}} %q\n", execPath)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	_, err = runnerClient.Start(ctx, execPath)
	cancel()
	gomega.Ω(err).Should(gomega.BeNil())

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	_, err = runnerClient.Health(ctx)
	cancel()
	gomega.Ω(err).Should(gomega.BeNil())

	color.Outf("{{green}}getting URIs{{/}}\n")
	var uris []string
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	uris, err = runnerClient.URIs(ctx)
	cancel()
	gomega.Ω(err).Should(gomega.BeNil())

	color.Outf("{{green}}creating subnet-cli client{{/}}\n")
	cli, err = client.New(client.Config{
		URI:          uris[0],
		PollInterval: time.Second,
	})
	gomega.Ω(err).Should(gomega.BeNil())

	k, err = key.NewSoft(9999999, key.WithPrivateKeyEncoded(key.EwoqPrivateKey))
	gomega.Ω(err).Should(gomega.BeNil())
})

var _ = ginkgo.AfterSuite(func() {
	color.Outf("{{red}}shutting down cluster{{/}}\n")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	_, err := runnerClient.Stop(ctx)
	cancel()
	gomega.Ω(err).Should(gomega.BeNil())

	color.Outf("{{red}}shutting down client{{/}}\n")
	err = runnerClient.Close()
	gomega.Ω(err).Should(gomega.BeNil())
})

var subnetID = ids.Empty

var _ = ginkgo.Describe("[CreateSubnet/CreateBlockchain]", func() {
	ginkgo.It("can issue CreateSubnetTx", func() {
		balance, err := cli.P().Balance(context.Background(), k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee(context.Background())
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
			curBal, err := cli.P().Balance(context.Background(), k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})

	ginkgo.It("can add subnet/validators", func() {
		nodeID, _, err := cli.Info().Client().GetNodeID(context.Background())
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
				ids.EmptyNodeID,
				time.Now(),
				time.Now(),
				1000,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrEmptyID.Error()))
		})

		ginkgo.By("fails to add an invalid subnet as a validator, when nodeID isn't validating the primary network", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddSubnetValidator(
				ctx,
				k,
				subnetID,
				ids.GenerateTestNodeID(),
				time.Now().Add(30*time.Second),
				time.Now().Add(2*24*time.Hour),
				1000,
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrNotValidatingPrimaryNetwork.Error()))
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

		ginkgo.By("fails to add duplicate validator", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err = cli.P().AddValidator(
				ctx,
				k,
				nodeID,
				time.Now().Add(30*time.Second),
				time.Now().Add(5*24*time.Hour),
				client.WithStakeAmount(2*units.KiloAvax),
				// ref. "genesis/genesis_local.go".
				client.WithRewardShares(30000), // 3%
			)
			cancel()
			gomega.Ω(err.Error()).Should(gomega.Equal(client.ErrAlreadyValidator.Error()))
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

		ginkgo.Skip("TODO: once we have a testable spaces VM in public")

		balance, err := cli.P().Balance(context.Background(), k)
		gomega.Ω(err).Should(gomega.BeNil())
		feeInfo, err := cli.Info().Client().GetTxFee(context.Background())
		gomega.Ω(err).Should(gomega.BeNil())
		blkChainFee := uint64(feeInfo.CreateBlockchainTxFee)
		expectedBalance := balance - blkChainFee

		ginkgo.By("returns a tx-fee deducted balance", func() {
			curBal, err := cli.P().Balance(context.Background(), k)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(curBal).Should(gomega.Equal(expectedBalance))
		})
	})
})
