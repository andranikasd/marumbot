# Every target runs inside a container: Docker is the only prerequisite.
GO_IMAGE  := golang:1.25
GO_ALPINE := golang:1.25-alpine
RUN       := docker run --rm -v $(PWD):/src -w /src -v marum-gomod:/go/pkg/mod

.DEFAULT_GOAL := help

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up: ## Start everything: postgres, the app, and the observability stack
	docker compose up -d --build

up-core: ## Start only postgres and the application
	docker compose up -d --build postgres marum

load: ## Run the k6 load profile against the local stack
	docker compose --profile load run --rm k6

grafana: ## Open Grafana
	@echo "http://127.0.0.1:3000 - datasources and dashboards are provisioned"

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

# --- migrations ---------------------------------------------------------
# goose is baked into a small image so migration commands are fast and need
# no network after the first build.
GOOSE_IMAGE := marum-goose:3.24.1
GOOSE := docker run --rm --network marum_default \
	-v $(PWD)/migrations:/migrations \
	-e GOOSE_DRIVER=postgres \
	-e GOOSE_DBSTRING="postgres://marum:marum@postgres:5432/marum?sslmode=disable" \
	$(GOOSE_IMAGE)

goose-image: ## Build the migration runner image
	docker build -q -f deploy/goose.Dockerfile -t $(GOOSE_IMAGE) . >/dev/null

migrate: goose-image ## Apply pending migrations
	$(GOOSE) up

migrate-down: goose-image ## Roll back one migration
	$(GOOSE) down

migrate-status: goose-image ## Show which migrations are applied
	$(GOOSE) status

migrate-check: goose-image ## Prove the newest migration is reversible: up, down, up
	$(GOOSE) up && $(GOOSE) down && $(GOOSE) up

seed: ## Load demo data for local development
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U marum -d marum < migrations/seed.sql

admin-password: ## Print a value for MARUM_ADMIN_PASSWORD_HASH
	@$(RUN) $(GO_ALPINE) go run ./cmd/marum -hash-password

shell: ## psql into the local database
	docker compose exec postgres psql -U marum -d marum

.PHONY: help up down reset logs test test-short vet lint fmt tidy shell \
	goose-image migrate migrate-down migrate-status migrate-check seed admin-password up-core load grafana
