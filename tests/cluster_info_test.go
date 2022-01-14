// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClusterInfo(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "testclusterinfo.yaml")

	ci := ClusterInfo{
		URIs: []string{"http://localhost:5000"},
		PID:  os.Getpid(),
	}
	if err := ci.Save(p); err != nil {
		t.Fatal(err)
	}

	ci2, err := LoadClusterInfo(p)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ci, ci2) {
		t.Fatalf("unexpected %+v, expected %+v", ci2, ci)
	}
}
