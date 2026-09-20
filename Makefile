.PHONY: build build-all test tidy restart stop

build:
	cd web && npm install && npm run build
	CGO_ENABLED=0 go build -o mockflow .

build-all: web/dist/index.html
	cd web && npm install && npm run build
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/mockflow-darwin-arm64 .
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/mockflow-linux-amd64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/mockflow-windows-amd64.exe .

test:
	CGO_ENABLED=0 go test ./...

tidy:
	go mod tidy

restart:
	./mockflow restart

stop:
	./mockflow stop
