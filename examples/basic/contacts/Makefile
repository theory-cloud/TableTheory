SHELL := /bin/bash
REPO_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))../../..)
ROOT_GOTOOLCHAIN := $(shell awk '/^toolchain /{print $$2; exit}' $(REPO_ROOT)/go.mod)
DYNAMODB_ENDPOINT ?= http://localhost:8000
GO_ENV := AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 DYNAMODB_ENDPOINT=$(DYNAMODB_ENDPOINT) GOTOOLCHAIN=$(ROOT_GOTOOLCHAIN)
COMPOSE ?= docker compose

.PHONY: docker-up docker-down run test fmt lint coverage dev-setup dev setup tables-create tables-delete clean

run:
	$(GO_ENV) go run .

test:
	$(GO_ENV) go test ./...

fmt:
	gofmt -w .

lint:
	$(GO_ENV) go vet ./...

coverage:
	$(GO_ENV) go test ./... -coverprofile=coverage.out

setup tables-create:
	@echo "Tables are created by the example on startup."

dev-setup: docker-up setup

dev: run

docker-up:
	@if ( cd "$(REPO_ROOT)" && GOTOOLCHAIN=$(ROOT_GOTOOLCHAIN) AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 go run ./internal/dynamodbprobe/cmd/dynamodb-local-check -endpoint $(DYNAMODB_ENDPOINT) ); then \
		echo "DynamoDB Local already available at $(DYNAMODB_ENDPOINT)"; \
	else \
		echo "DynamoDB Local not detected at $(DYNAMODB_ENDPOINT); starting it with Docker Compose"; \
		$(COMPOSE) up -d; \
	fi

docker-down clean:
	$(COMPOSE) down

tables-delete:
	@echo "DynamoDB Local is in-memory; use 'make docker-down' to discard local example state."
