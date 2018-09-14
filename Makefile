.PHONY: prep

OUT=bin/proxy

build: ${OUT}

bin/proxy: prep
	GOOS=darwin GOARCH=amd64 go build -o bin/proxy-macos64 cmd/proxy.go
	GOOS=linux GOARCH=amd64 go build -o bin/proxy-linux cmd/proxy.go
	GOOS=windows GOARCH=amd64 go build -o bin/proxy.exe cmd/proxy.go

prep:
	mkdir -p bin

clean:
	rm -r bin