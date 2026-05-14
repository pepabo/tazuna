.PHONY: all format test test-integration test-e2e test-all build devenv-create devenv-destroy install lint

all: format test build lint

format:
	go fmt ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

test-e2e: build devenv-create
	go test -tags=e2e -count=1 ./test/e2e/...

test-all: test test-integration test-e2e

build:
	go build .

lint: 
	golangci-lint run

install: build
	sudo mv tazuna /usr/local/bin

devenv-create:
	@if ! kind get clusters | grep -q '^tazuna$$'; then \
		kind create cluster --name tazuna; \
	else \
		echo "KinD cluster 'tazuna' already exists"; \
		kubectl config use-context kind-tazuna; \
	fi
	kubectl cluster-info --context kind-tazuna

devenv-destroy:
	kind delete cluster --name tazuna

