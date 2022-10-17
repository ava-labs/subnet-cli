#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if ! [[ "$0" =~ scripts/build.release.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

# https://goreleaser.com/install/
go install -v github.com/goreleaser/goreleaser@latest

# Set the CGO flags to use the portable version of BLST
#
# We use "export" here instead of just setting a bash variable because we need
# to pass this flag to all child processes spawned by the shell.
export CGO_CFLAGS="-O -D__BLST_PORTABLE__"

# e.g.,
# git tag 1.0.0
goreleaser release \
--config .goreleaser.yml \
--skip-announce \
--skip-publish
