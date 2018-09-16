.PHONY: prep

OUT=bin/phantom.exe bin/phantom-linux bin/phantom-macos

build: prep ${OUT}

bin/phantom.exe:
	GOOS=windows GOARCH=amd64 go build -o bin/phantom.exe cmd/proxy.go

bin/phantom-macos:
	GOOS=darwin GOARCH=amd64 go build -o bin/phantom-macos cmd/proxy.go

bin/phantom-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/phantom-linux cmd/proxy.go

prep:
	mkdir -p bin

clean:
	rm -r bin