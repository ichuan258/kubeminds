# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest-setup.
ENVTEST_K8S_VERSION = 1.28.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

## Tool Versions (Hooks)
GOLANGCI_LINT_VERSION ?= v1.64.8
GOSEC_VERSION ?= v2.23.0
GOVULNCHECK_VERSION ?= v1.1.4
GITLEAKS_VERSION ?= v8.30.0

##@ General

# The help target prints out all targets with their descriptions organized by category
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Security

.PHONY: encrypt-key
encrypt-key: ## Encrypt an LLM API key for storage in config.yaml. Usage: make encrypt-key KEY=sk-xxxx
	@if [ -z "$(KEY)" ]; then \
		echo "Usage: make encrypt-key KEY=<your-api-key>"; \
		echo "Also requires: export KUBEMINDS_MASTER_KEY=<64-hex-chars>"; \
		exit 1; \
	fi
	@go run ./cmd/tools/encryptkey/main.go "$(KEY)"

##@ Local Dev Environment

.PHONY: dev-redis-start
dev-redis-start: ## Start a local Redis container for L2 event store testing.
	docker run -d --name kubeminds-redis -p 6379:6379 redis:7-alpine
	@echo "Redis started on localhost:6379. Stop with: make dev-stop"

.PHONY: dev-postgres-start
dev-postgres-start: ## Start a local PostgreSQL+pgvector container for L3 knowledge base testing.
	docker run -d --name kubeminds-postgres \
		-e POSTGRES_USER=kubeminds \
		-e POSTGRES_PASSWORD=kubeminds \
		-e POSTGRES_DB=kubeminds \
		-p 5432:5432 \
		pgvector/pgvector:pg16
	@echo "PostgreSQL started on localhost:5432 (user=kubeminds, db=kubeminds). Stop with: make dev-stop"

.PHONY: dev-stop
dev-stop: ## Stop and remove local dev containers (Redis + PostgreSQL).
	docker rm -f kubeminds-redis kubeminds-postgres 2>/dev/null || true
	@echo "Dev containers stopped."

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

.PHONY: hook-install
hook-install: ## Install git hooks (pre-commit/pre-push).
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit .githooks/pre-push
	chmod +x .claude/hooks/check-fast.sh .claude/hooks/check-full.sh
	@echo "Git hooks installed: $$(git config core.hooksPath)"

.PHONY: hook-tools
hook-tools: $(LOCALBIN) ## Install hook toolchain to ./bin.
	GOBIN=$(LOCALBIN) GOPROXY=$${GOPROXY:-https://proxy.golang.org,direct} go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(LOCALBIN) GOPROXY=$${GOPROXY:-https://proxy.golang.org,direct} go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	GOBIN=$(LOCALBIN) GOPROXY=$${GOPROXY:-https://proxy.golang.org,direct} go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	GOBIN=$(LOCALBIN) GOPROXY=$${GOPROXY:-https://proxy.golang.org,direct} go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

.PHONY: hook-fast
hook-fast: ## Run fast local gate (format/lint/secrets/build).
	@bash .claude/hooks/check-fast.sh

hook-full: ## Run full local gate (lint/test/security/vuln/secrets).
	@bash .claude/hooks/check-full.sh

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/manager/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.2.1
CONTROLLER_TOOLS_VERSION ?= v0.13.0
ENVTEST_VERSION ?= release-0.16

KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)
