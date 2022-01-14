
## Local test `runner`

- Sets up local cluster with https://github.com/ava-labs/avalanche-network-runner.
- Used for e2e/conformance/stress testing in local development environments.
- Requires `github.com/ava-labs/avalanchego` binaries.

To build latest `avalanchego`:

```bash
cd ${HOME}/go/src/github.com/ava-labs/avalanchego
rm -rf ./build
./scripts/build.sh
```

Build and run local test `runner`:

```bash
cd ${HOME}/go/src/github.com/ava-labs/snow-machine
pushd ./tests/runner
go build -o /tmp/runner -v .
popd

/tmp/runner \
--avalanchego-path ${HOME}/go/src/github.com/ava-labs/avalanchego/build/avalanchego \
--output-path /tmp/snow-machine.runner.yml
```

Or use Go directly to run `runner`:

```bash
cd ${HOME}/go/src/github.com/ava-labs/snow-machine/tests/runner
go run . \
--avalanchego-path ${HOME}/go/src/github.com/ava-labs/avalanchego/build/avalanchego \
--output-path /tmp/snow-machine.runner.yml
```

Then see the output (e.g., cluster endpoints):

```bash
cat /tmp/snow-machine.runner.yml
```
