SHELL=/bin/bash
.PHONY: prep build build-target clean test test-e2e test-all fuzz corpus drift

OUT=bin/phantom-windows.exe bin/phantom-windows-32bit.exe bin/phantom-macos bin/phantom-macos-arm8 bin/phantom-linux bin/phantom-linux-arm5 bin/phantom-linux-arm6 bin/phantom-linux-arm7 bin/phantom-linux-arm8
CMDSRC=phantom.go

build: prep ${OUT}

bin/phantom-windows.exe:
	pushd cmd && \
	GOOS=windows GOARCH=amd64 go build -o ../bin/phantom-windows.exe ${CMDSRC} && \
	popd

bin/phantom-windows-32bit.exe:
	pushd cmd && \
	GOOS=windows GOARCH=386 go build -o ../bin/phantom-windows-32bit.exe ${CMDSRC} && \
	popd

bin/phantom-macos:
	pushd cmd && \
	GOOS=darwin GOARCH=amd64 go build -o ../bin/phantom-macos ${CMDSRC} && \
	popd

bin/phantom-macos-arm8:
	pushd cmd && \
	GOOS=darwin GOARCH=arm64 go build -o ../bin/phantom-macos-arm8 ${CMDSRC} && \
	popd

bin/phantom-linux:
	pushd cmd && \
	GOOS=linux GOARCH=amd64 go build -o ../bin/phantom-linux ${CMDSRC} && \
	popd

bin/phantom-linux-arm5:
	pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=5 go build -o ../bin/phantom-linux-arm5 ${CMDSRC} && \
	popd

bin/phantom-linux-arm6:
	pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=6 go build -o ../bin/phantom-linux-arm6 ${CMDSRC} && \
	popd

bin/phantom-linux-arm7:
	pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=7 go build -o ../bin/phantom-linux-arm7 ${CMDSRC} && \
	popd

bin/phantom-linux-arm8:
	pushd cmd && \
	GOOS=linux GOARCH=arm64 go build -o ../bin/phantom-linux-arm8 ${CMDSRC} && \
	popd

prep:
	mkdir -p bin

clean:
	rm -rf bin

# Unit tests over the cross-version corpus. Fast, no deps.
test:
	go test ./...

# End-to-end tests against a real phantom subprocess.
# Needs `npm ci` in test/compat/node first for the `node` tag to do anything;
# without it those cases skip rather than fail.
test-e2e:
	go test -tags='e2e slow node' ./test/compat/... -timeout=20m

test-all: test test-e2e

fuzz:
	go test ./internal/proto/ -run=Fuzz -fuzz=FuzzReadUnconnectedPing -fuzztime=60s

# Regenerates the captured pong corpus from real bedrock-protocol servers, one
# per supported Minecraft version. Dev-time only: CI replays the committed
# bytes and never runs this. Commit the diff.
corpus:
	cd internal/corpus/gen && npm install --no-audit --no-fund && node capture.js

# Reports Minecraft versions that bedrock-protocol supports but this repo does
# not yet test.
drift:
	cd internal/corpus/gen && node drift.js
