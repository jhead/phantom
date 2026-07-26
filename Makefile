SHELL=/bin/bash
.PHONY: prep

# linux-x86 = GOARCH=386 (iSH on iOS is x86 userspace; ARM/amd64 builds fail there)
OUT=bin/phantom-windows.exe bin/phantom-windows-32bit.exe bin/phantom-macos bin/phantom-macos-arm8 bin/phantom-linux bin/phantom-linux-x86 bin/phantom-linux-arm5 bin/phantom-linux-arm6 bin/phantom-linux-arm7 bin/phantom-linux-arm8
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

# 32-bit x86 for iSH (iOS) and other linux/386 environments.
bin/phantom-linux-x86:
	pushd cmd && \
	CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -o ../bin/phantom-linux-x86 ${CMDSRC} && \
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

test:
	go test ./...
