// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// runner uses "avalanche-network-runner" to set up a local network.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/ava-labs/avalanche-network-runner/api"
	"github.com/ava-labs/avalanche-network-runner/local"
	"github.com/ava-labs/avalanche-network-runner/network"
	"github.com/ava-labs/avalanche-network-runner/network/node"
	"github.com/ava-labs/avalanchego/ids"
	avago_constants "github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/subnet-cli/tests"
	formatter "github.com/onsi/ginkgo/v2/formatter"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:        "subnet-cli.runner",
	Short:      "avalanche-network-runner wrapper",
	SuggestFor: []string{"subnet-cli-runner"},
	RunE:       runFunc,
}

func init() {
	cobra.EnablePrefixMatching = true
}

var (
	avalancheGoBinPath string
	dataDir            string
	whitelistedSubnets string
	outputPath         string
)

func init() {
	dir, err := ioutil.TempDir(os.TempDir(), "subnet-cli-runner")
	if err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().StringVar(
		&avalancheGoBinPath,
		"avalanchego-path",
		"",
		"avalanchego binary path",
	)

	rootCmd.PersistentFlags().StringVar(
		&dataDir,
		"data-dir",
		dir,
		"avalanchego binary path",
	)

	// TODO: remove this once we have dynamic whitelisting API
	rootCmd.PersistentFlags().StringVar(
		&whitelistedSubnets,
		"whitelisted-subnets",
		"",
		"a list of subnet tx IDs to whitelist",
	)

	rootCmd.PersistentFlags().StringVar(
		&outputPath,
		"output-path",
		"",
		"output YAML path to write local cluster information",
	)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "runner failed %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runFunc(cmd *cobra.Command, args []string) error {
	return run(avalancheGoBinPath, outputPath)
}

func run(avalancheGoBinPath string, outputPath string) (err error) {
	lc := newLocalNetwork(avalancheGoBinPath, outputPath)

	go lc.start()
	select {
	case <-lc.readyc:
		outf("{{green}}cluster is ready, waiting for signal/error{{/}}\n")
	case s := <-lc.sigc:
		outf("{{red}}received signal %v before ready, shutting down{{/}}\n", s)
		lc.shutdown()
		return nil
	}
	select {
	case s := <-lc.sigc:
		outf("{{red}}received signal %v, shutting down{{/}}\n", s)
	case err = <-lc.errc:
		outf("{{red}}received error %v, shutting down{{/}}\n", err)
	}

	lc.shutdown()
	return err
}

type localNetwork struct {
	logger logging.Logger

	cfg network.Config

	binPath    string
	outputPath string

	nw network.Network

	nodes     map[string]node.Node
	nodeNames []string
	nodeIDs   map[string]string
	uris      map[string]string
	apiClis   map[string]api.Client

	avaxAssetID ids.ID
	xChainID    ids.ID
	pChainID    ids.ID
	cChainID    ids.ID

	readyc          chan struct{} // closed when local network is ready/healthy
	readycCloseOnce sync.Once

	sigc  chan os.Signal
	stopc chan struct{}
	donec chan struct{}
	errc  chan error
}

func newLocalNetwork(
	avalancheGoBinPath string,
	outputPath string,
) *localNetwork {
	lcfg, err := logging.DefaultConfig()
	if err != nil {
		panic(err)
	}
	logFactory := logging.NewFactory(lcfg)
	logger, err := logFactory.Make("main")
	if err != nil {
		panic(err)
	}

	cfg := local.NewDefaultConfig(avalancheGoBinPath)
	nodeNames := make([]string, len(cfg.NodeConfigs))
	for i := range cfg.NodeConfigs {
		nodeName := fmt.Sprintf("node%d", i+1)

		nodeNames[i] = nodeName
		cfg.NodeConfigs[i].Name = nodeName

		// need to whitelist subnet ID to create custom VM chain
		// ref. vms/platformvm/createChain
		cfg.NodeConfigs[i].ConfigFile = []byte(fmt.Sprintf(`{
	"network-peer-list-gossip-frequency":"250ms",
	"network-max-reconnect-delay":"1s",
	"public-ip":"127.0.0.1",
	"health-check-frequency":"2s",
	"api-admin-enabled":true,
	"api-ipcs-enabled":true,
	"index-enabled":true,
	"log-display-level":"INFO",
	"log-level":"INFO",
	"log-dir":"%s",
	"db-dir":"%s",
	"whitelisted-subnets":"%s"
}`,
			filepath.Join(dataDir, nodeName, "logs"),
			filepath.Join(dataDir, nodeName, "db-dir"),
			whitelistedSubnets,
		))
		wr := &writer{
			c:    colors[i%len(cfg.NodeConfigs)],
			name: nodeName,
			w:    os.Stdout,
		}
		cfg.NodeConfigs[i].ImplSpecificConfig = local.NodeConfig{
			BinaryPath: avalancheGoBinPath,
			Stdout:     wr,
			Stderr:     wr,
		}
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	return &localNetwork{
		logger: logger,

		cfg: cfg,

		binPath:    avalancheGoBinPath,
		outputPath: outputPath,

		nodeNames: nodeNames,
		nodeIDs:   make(map[string]string),
		uris:      make(map[string]string),
		apiClis:   make(map[string]api.Client),

		pChainID: avago_constants.PlatformChainID,

		readyc: make(chan struct{}),
		sigc:   sigc,
		stopc:  make(chan struct{}),
		donec:  make(chan struct{}),
		errc:   make(chan error, 1),
	}
}

func (lc *localNetwork) start() {
	defer func() {
		close(lc.donec)
	}()

	outf("{{blue}}{{bold}}create and run local network with data dir %q{{/}}\n", dataDir)
	nw, err := local.NewNetwork(lc.logger, lc.cfg)
	if err != nil {
		lc.errc <- err
		return
	}
	lc.nw = nw

	if err := lc.waitForHealthy(); err != nil {
		lc.errc <- err
		return
	}

	if err := lc.writeOutput(); err != nil {
		lc.errc <- err
		return
	}
}

const (
	healthyWait   = 2 * time.Minute
	txConfirmWait = time.Minute
)

var errAborted = errors.New("aborted")

func (lc *localNetwork) waitForHealthy() error {
	outf("{{blue}}{{bold}}waiting for all nodes to report healthy...{{/}}\n")

	ctx, cancel := context.WithTimeout(context.Background(), healthyWait)
	defer cancel()
	hc := lc.nw.Healthy(ctx)
	select {
	case <-lc.stopc:
		return errAborted
	case <-ctx.Done():
		return ctx.Err()
	case err := <-hc:
		if err != nil {
			return err
		}
	}

	nodes, err := lc.nw.GetAllNodes()
	if err != nil {
		return err
	}
	lc.nodes = nodes

	for name, node := range nodes {
		nodeID := node.GetNodeID().PrefixedString(avago_constants.NodeIDPrefix)
		lc.nodeIDs[name] = nodeID

		uri := fmt.Sprintf("http://%s:%d", node.GetURL(), node.GetAPIPort())
		lc.uris[name] = uri

		lc.apiClis[name] = node.GetAPIClient()
		outf("{{cyan}}%s: node ID %q, URI %q{{/}}\n", name, nodeID, uri)

		if lc.avaxAssetID == ids.Empty {
			avaxDesc, err := lc.apiClis[name].XChainAPI().GetAssetDescription("AVAX")
			if err != nil {
				return fmt.Errorf("%q failed to get AVAX asset ID %w", name, err)
			}
			lc.avaxAssetID = avaxDesc.AssetID
		}
		if lc.xChainID == ids.Empty {
			xChainID, err := lc.apiClis[name].InfoAPI().GetBlockchainID("X")
			if err != nil {
				return fmt.Errorf("%q failed to get blockchain ID %w", name, err)
			}
			lc.xChainID = xChainID
		}
		if lc.cChainID == ids.Empty {
			cChainID, err := lc.apiClis[name].InfoAPI().GetBlockchainID("C")
			if err != nil {
				return fmt.Errorf("%q failed to get blockchain ID %w", name, err)
			}
			lc.cChainID = cChainID
		}
	}

	lc.readycCloseOnce.Do(func() {
		close(lc.readyc)
	})
	return nil
}

func (lc *localNetwork) getURIs() []string {
	uris := make([]string, 0, len(lc.uris))
	for _, u := range lc.uris {
		uris = append(uris, u)
	}
	sort.Strings(uris)
	return uris
}

func (lc *localNetwork) writeOutput() error {
	pid := os.Getpid()
	outf("{{blue}}{{bold}}writing output %q with PID %d{{/}}\n", lc.outputPath, pid)
	ci := tests.ClusterInfo{
		URIs:        lc.getURIs(),
		NetworkID:   1337, // same as network-runner genesis
		AvaxAssetID: lc.avaxAssetID.String(),
		XChainID:    lc.xChainID.String(),
		PChainID:    lc.pChainID.String(),
		CChainID:    lc.cChainID.String(),
		PID:         pid,
		DataDir:     dataDir,
	}
	err := ci.Save(lc.outputPath)
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(lc.outputPath)
	if err != nil {
		return err
	}
	outf("\n{{blue}}$ cat %s:{{/}}\n%s\n", lc.outputPath, string(b))
	return nil
}

func (lc *localNetwork) shutdown() {
	close(lc.stopc)
	serr := lc.nw.Stop(context.Background())
	<-lc.donec
	outf("{{red}}{{bold}}terminated network{{/}} (error %v)\n", serr)
}

// https://github.com/onsi/ginkgo/blob/v2.0.0/formatter/formatter.go#L52-L73
func outf(format string, args ...interface{}) {
	s := formatter.F(format, args...)
	fmt.Fprint(formatter.ColorableStdOut, s)
}

type writer struct {
	c    string
	name string
	w    io.Writer
}

// https://github.com/onsi/ginkgo/blob/v2.0.0/formatter/formatter.go#L52-L73
var colors = []string{
	"{{green}}",
	"{{orange}}",
	"{{blue}}",
	"{{magenta}}",
	"{{cyan}}",
}

func (wr *writer) Write(p []byte) (n int, err error) {
	s := formatter.F(wr.c+"[%s]{{/}}	", wr.name)
	fmt.Fprint(formatter.ColorableStdOut, s)
	return wr.w.Write(p)
}
