#!/usr/bin/env bash

export GOPRIVATE=github.com/lamina1/subnet-cli
echo $(env | grep GOPRIVATE)

set -o errexit
set -o nounset
set -o pipefail

if ! [[ "$0" =~ scripts/build.only.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

go build -v .

# https://goreleaser.com/install/
# go install -v github.com/goreleaser/goreleaser@latest

# # e.g.,
# # git tag 1.0.0
# goreleaser release \
# --config .goreleaser.yml \
# --skip-announce \
# --skip-publish
