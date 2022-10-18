#!/usr/bin/env bash
set -e

if ! [[ "$0" =~ scripts/tests.unit.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

# Set the CGO flags to use the portable version of BLST                                                                 
#                                                                                                                       
# We use "export" here instead of just setting a bash variable because we need                                          
# to pass this flag to all child processes spawned by the shell.                                                        
export CGO_CFLAGS="-O -D__BLST_PORTABLE__"

go test -v -race -timeout="3m" -coverprofile="coverage.out" -covermode="atomic" $(go list ./... | grep -v tests)
