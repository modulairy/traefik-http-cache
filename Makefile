.PHONY: check fmt lint test integration up down

# Keep the plugin package at the module root, as in traefik/plugindemo.
check: lint test

fmt:
	gofmt -w *.go internal/store/*.go internal/testbackend/*.go

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

test:
	go test -race -count=1 ./...

up:
	docker compose -f infra/compose.yml up -d --build

# Start the stack and wait for router readiness before using this target.
integration:
	docker compose -f infra/compose.yml --profile test run --rm test

down:
	docker compose -f infra/compose.yml down
