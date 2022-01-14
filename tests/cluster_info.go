// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package tests defines common test primitives.
package tests

import (
	"io/ioutil"

	"sigs.k8s.io/yaml"
)

// ClusterInfo represents the local cluster information.
type ClusterInfo struct {
	URIs        []string `json:"uris"`
	NetworkID   uint32   `json:"networkId"`
	AvaxAssetID string   `json:"avaxAssetId"`
	XChainID    string   `json:"xChainId"`
	PChainID    string   `json:"pChainId"`
	CChainID    string   `json:"cChainId"`
	PID         int      `json:"pid"`
	DataDir     string   `json:"dataDir"`
}

const fsModeWrite = 0o600

func (ci ClusterInfo) Save(p string) error {
	ob, err := yaml.Marshal(ci)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(p, ob, fsModeWrite)
}

// LoadClusterInfo loads the cluster info YAML file
// to parse it into "ClusterInfo".
func LoadClusterInfo(p string) (ClusterInfo, error) {
	ob, err := ioutil.ReadFile(p)
	if err != nil {
		return ClusterInfo{}, err
	}
	info := new(ClusterInfo)
	if err = yaml.Unmarshal(ob, info); err != nil {
		return ClusterInfo{}, err
	}
	return *info, nil
}
