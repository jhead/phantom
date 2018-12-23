.PHONY: prep

OUT=bin/phantom.exe bin/phantom-macos bin/phantom-linux bin/phantom-linux-arm6 bin/phantom-linux-arm7

build: prep ${OUT}

bin/phantom.exe:
	GOOS=windows GOARCH=amd64 go build -o bin/phantom.exe cmd/proxy.go

bin/phantom-macos:
	GOOS=darwin GOARCH=amd64 go build -o bin/phantom-macos cmd/proxy.go

bin/phantom-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/phantom-linux cmd/proxy.go

bin/phantom-linux-arm6:
	GOOS=linux GOARCH=arm GOARM=6 go build -o bin/phantom-linux-arm6 cmd/proxy.go

bin/phantom-linux-arm7:
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/phantom-linux-arm7 cmd/proxy.go

prep:
	mkdir -p bin

clean:
	rm -r bin

