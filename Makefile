SHELL=/bin/bash
.PHONY: prep

# ARM note:
#   arm5/arm6/arm7 = 32-bit (GOARCH=arm + GOARM)
#   arm8/arm64     = 64-bit aarch64 (GOARCH=arm64). "arm8" does NOT mean
#                    "any ARMv8 Raspberry Pi" — most Pi OS installs are still
#                    32-bit and need arm7 (or arm6 on Pi Zero / Pi 1).
# linux-x86 = GOARCH=386 (iSH on iOS is x86 userspace; ARM/amd64 builds fail there)
OUT=bin/phantom-windows.exe bin/phantom-windows-32bit.exe bin/phantom-macos bin/phantom-macos-arm8 bin/phantom-linux bin/phantom-linux-x86 bin/phantom-linux-arm5 bin/phantom-linux-arm6 bin/phantom-linux-arm7 bin/phantom-linux-arm8 bin/phantom-linux-arm64
CMDSRC=phantom.go

build: prep ${OUT}

bin/phantom-windows.exe:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../bin/phantom-windows.exe ${CMDSRC} && \
	popd

bin/phantom-windows-32bit.exe:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -o ../bin/phantom-windows-32bit.exe ${CMDSRC} && \
	popd

bin/phantom-macos:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o ../bin/phantom-macos ${CMDSRC} && \
	popd

bin/phantom-macos-arm8:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ../bin/phantom-macos-arm8 ${CMDSRC} && \
	popd

bin/phantom-linux:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../bin/phantom-linux ${CMDSRC} && \
	popd

# Termux on Android uses these linux/arm* targets (not GOOS=android).
# 32-bit x86 for iSH (iOS) and other linux/386 environments.
bin/phantom-linux-x86:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -o ../bin/phantom-linux-x86 ${CMDSRC} && \
	popd

bin/phantom-linux-arm5:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5 go build -o ../bin/phantom-linux-arm5 ${CMDSRC} && \
	popd

bin/phantom-linux-arm6:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -o ../bin/phantom-linux-arm6 ${CMDSRC} && \
	popd

bin/phantom-linux-arm7:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o ../bin/phantom-linux-arm7 ${CMDSRC} && \
	popd

# 64-bit ARM / aarch64 only (64-bit Raspberry Pi OS, etc.)
# GOARM64=v8.0 keeps the baseline compatible with Pi 3/4 (no LSE requirement).
bin/phantom-linux-arm8:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build -o ../bin/phantom-linux-arm8 ${CMDSRC} && \
	popd

# Clearer alias for arm8 (same aarch64 binary).
bin/phantom-linux-arm64: bin/phantom-linux-arm8
	cp -f bin/phantom-linux-arm8 bin/phantom-linux-arm64

prep:
	mkdir -p bin

clean:
	rm -rf bin

test:
	go test ./...
