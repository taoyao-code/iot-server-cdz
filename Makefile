SHELL := /bin/bash
APP := iot-server
PKG := ./...

.PHONY: all tidy fmt vet build run test clean lint

all: tidy fmt vet build

tidy:
	go mod tidy

fmt:
	go fmt $(PKG)

vet:
	go vet $(PKG)

build:
	GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) go build -o bin/$(APP) ./cmd/server

run:
	IOT_CONFIG=./configs/example.yaml go run ./cmd/server

test:
	go test -race $(PKG)

clean:
	rm -rf bin

.PHONY: compose-up compose-down
compose-up:
	docker compose up -d

compose-down:
	docker compose down -v


