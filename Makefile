.PHONY: prep

OUT=bin/phantom.exe bin/phantom-macos bin/phantom-linux bin/phantom-linux-arm5 bin/phantom-linux-arm6 bin/phantom-linux-arm7

build: prep ${OUT}

bin/phantom.exe:
	@pushd cmd && \
	cmd GOOS=windows GOARCH=amd64 go build -o ../bin/phantom.exe proxy.go && \
	popd

bin/phantom-macos:
	@pushd cmd && \
	GOOS=darwin GOARCH=amd64 go build -o ../bin/phantom-macos proxy.go && \
	popd

bin/phantom-linux:
	@pushd cmd && \
	GOOS=linux GOARCH=amd64 go build -o ../bin/phantom-linux proxy.go && \
	@popd

bin/phantom-linux-arm5:
	@pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=5 go build -o ../bin/phantom-linux-arm5 proxy.go && \
	@popd

bin/phantom-linux-arm6:
	@pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=6 go build -o ../bin/phantom-linux-arm6 proxy.go && \
	@popd

bin/phantom-linux-arm7:
	@pushd cmd && \
	GOOS=linux GOARCH=arm GOARM=7 go build -o ../bin/phantom-linux-arm7 proxy.go && \
	@popd

prep:
	mkdir -p bin

clean:
	rm -rf bin

