# Every target runs inside a container: Docker is the only prerequisite.
GO_IMAGE  := golang:1.27
GO_ALPINE := golang:1.27-alpine
TF_IMAGE  := hashicorp/terraform:1.13.3
RUN       := docker run --rm -v $(PWD):/src -w /src -v marum-gomod:/go/pkg/mod

# Terraform reads every credential from the environment; nothing is passed on a
# command line, where it would land in shell history and in `ps`.
TF_ENV := -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY \
	-e TF_VAR_cloudflare_api_token -e TF_VAR_cloudflare_account_id \
	-e TF_VAR_cloudflare_zone_id -e TF_VAR_database
TF := docker run --rm -i $(TF_ENV) -v $(PWD)/deploy/terraform:/tf -w /tf $(TF_IMAGE)

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

# Pinned, not :latest. CI pins a version; if this floats, local and CI disagree
# and the disagreement only surfaces in a pull request.
GOLANGCI_VERSION := v2.13.1

lint: ## gofumpt and golangci-lint
	$(RUN) golangci/golangci-lint:$(GOLANGCI_VERSION) golangci-lint run --timeout 5m

# Pinned for the same reason golangci-lint is: a floating formatter makes local
# and CI disagree about what formatted means.
GOFUMPT_VERSION := v0.7.0

fmt: ## Format with gofumpt
	$(RUN) $(GO_ALPINE) sh -c 'go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION) -l -w .'

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
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U marum -d marum < deploy/seed.sql

# -i so the container actually receives stdin. Without it the prompt reads EOF
# and the command fails before it can ask for anything.
admin-password: ## Print a value for MARUM_ADMIN_PASSWORD_HASH (reads a password on stdin)
	@$(RUN) -i -e MARUM_ADMIN_PASSWORD $(GO_ALPINE) go run ./cmd/marum -hash-password

# ENV picks the environment and the state file with it. There is no default:
# guessing which environment an infrastructure change lands in is not a mistake
# worth making convenient.
tf-init: ## Initialise Terraform for one environment (ENV=dev|production)
	@test -n "$(ENV)" || { echo "ENV=dev or ENV=production is required"; exit 1; }
	@test -n "$$TF_VAR_cloudflare_account_id" || { echo "TF_VAR_cloudflare_account_id is not set; see deploy/terraform/README.md"; exit 1; }
	$(TF) init -reconfigure \
		-backend-config=envs/$(ENV).backend.hcl \
		-backend-config="endpoints={\"s3\":\"https://$$TF_VAR_cloudflare_account_id.r2.cloudflarestorage.com\"}"

tf-plan: ## Show what would change (ENV=dev|production)
	@test -n "$(ENV)" || { echo "ENV=dev or ENV=production is required"; exit 1; }
	$(TF) plan -var-file=envs/$(ENV).tfvars

tf-apply: ## Apply infrastructure changes (ENV=dev|production)
	@test -n "$(ENV)" || { echo "ENV=dev or ENV=production is required"; exit 1; }
	$(TF) apply -var-file=envs/$(ENV).tfvars

tf-output: ## Print the wrangler.toml Hyperdrive binding for this environment
	$(TF) output -raw wrangler_hint

tf-fmt: ## Format the Terraform files
	$(TF) fmt -recursive

tf-validate: ## Check the Terraform configuration without contacting Cloudflare
	docker run --rm -v $(PWD)/deploy/terraform:/tf -w /tf $(TF_IMAGE) init -backend=false -input=false >/dev/null
	docker run --rm -v $(PWD)/deploy/terraform:/tf -w /tf $(TF_IMAGE) validate

shell: ## psql into the local database
	docker compose exec postgres psql -U marum -d marum

.PHONY: help up down reset logs test test-short vet lint fmt tidy shell \
	goose-image migrate migrate-down migrate-status migrate-check seed admin-password up-core load grafana \
	tf-init tf-plan tf-apply tf-output tf-fmt tf-validate
