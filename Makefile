SHELL := /usr/bin/env bash

BINARY ?= bin/kubesqueeze
IMAGE ?= kubesqueeze:dev
KIND_NODE_IMAGE ?= kindest/node:v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f

.PHONY: build test check e2e image clean

build:
	mkdir -p $(dir $(BINARY))
	go build -trimpath -o $(BINARY) ./cmd/kubesqueeze

test:
	go test ./...

check:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...

e2e: build
	KIND_NODE_IMAGE='$(KIND_NODE_IMAGE)' KUBESQUEEZE_BIN='$(abspath $(BINARY))' test/e2e/run.sh

image:
	docker build --build-arg VERSION=dev -t $(IMAGE) .

clean:
	rm -rf bin
