.PHONY: test build run-collector run-sim tidy

test:
	go test ./...

tidy:
	go mod tidy

build:
	mkdir -p bin
	go build -o bin/bmp-collector ./cmd/bmp-collector
	go build -o bin/bmp-simulator ./cmd/bmp-simulator

run-collector:
	go run ./cmd/bmp-collector -listen 0.0.0.0:11019 -http 127.0.0.1:18080

run-sim:
	go run ./cmd/bmp-simulator -collector 127.0.0.1:11019 -interval 5s
