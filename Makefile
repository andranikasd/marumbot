# Every target runs inside a container: Docker is the only prerequisite.
GO_IMAGE  := golang:1.25
GO_ALPINE := golang:1.25-alpine
RUN       := docker run --rm -v $(PWD):/src -w /src -v marum-gomod:/go/pkg/mod

.DEFAULT_GOAL := help

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up: ## Start postgres and the application
	docker compose up -d --build

down: ## Stop everything, keep the database volume
	docker compose down

reset: ## Stop and destroy the database volume
	docker compose down -v

logs: ## Follow the application log
	docker compose logs -f marum

test: ## Unit and golden tests, race detector on
	$(RUN) -e CGO_ENABLED=1 $(GO_IMAGE) go test -race -count=1 ./...

test-short: ## Tests without the race detector
	$(RUN) $(GO_ALPINE) go test -count=1 ./...

vet: ## go vet
	$(RUN) $(GO_ALPINE) go vet ./...

lint: ## gofumpt and golangci-lint
	$(RUN) golangci/golangci-lint:latest golangci-lint run --timeout 5m

fmt: ## Format with gofumpt
	$(RUN) $(GO_ALPINE) sh -c 'go run mvdan.cc/gofumpt@latest -l -w .'

tidy: ## go mod tidy
	$(RUN) $(GO_ALPINE) go mod tidy

shell: ## psql into the local database
	docker compose exec postgres psql -U marum -d marum

.PHONY: help up down reset logs test test-short vet lint fmt tidy shell
