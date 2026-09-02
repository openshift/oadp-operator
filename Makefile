# TOOL VERSIONS
# All version-related variables are defined here for easy maintenance
DEFAULT_VERSION := 99.0.0
VERSION ?= $(DEFAULT_VERSION) # the version of the operator
OPERATOR_SDK_VERSION ?= v1.35.0
ENVTEST_K8S_VERSION = 1.32 #refers to the version of kubebuilder assets to be downloaded by envtest binary # Kubernetes version from OpenShift 4.19.x
GOLANGCI_LINT_VERSION ?= v2.9.0
KUSTOMIZE_VERSION ?= v5.2.1
CONTROLLER_TOOLS_VERSION ?= v0.16.5
# Also defined in build/Dockerfile.catalog — keep in sync
OPM_VERSION ?= v1.68.0
BRANCH_VERSION = oadp-dev
PREVIOUS_CHANNEL ?= oadp-1.5
PREVIOUS_CHANNEL_GO_VERSION ?= 1.23
# Extract the toolchain directive from go.mod
GO_TOOLCHAIN_VERSION := $(shell grep -E "^toolchain" go.mod | awk '{print $$2}')

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS = "dev"
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL = "dev"
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# openshift.io/oadp-operator-bundle:$VERSION and openshift.io/oadp-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= openshift.io/oadp-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --extra-service-accounts "velero,non-admin-controller,oadp-vm-file-restore-controller-manager,oadp-kubevirt-datamover-controller-manager" --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-operator:latest

# TTL_DURATION defines the time-to-live for temporary images pushed to ttl.sh
# The maximum allowed value by ttl.sh is 24h. Default is 1h.
# You can override this with environment variable (e.g., export TTL_DURATION=4h)
TTL_DURATION ?= 1h

# HC_BACKUP_RESTORE_MODE is the mode of the HostedCluster to use for HCP tests.
HC_BACKUP_RESTORE_MODE ?= external # create, external, external-rosa
# HC_NAME is the name of the HostedCluster to use for HCP tests when HC_BACKUP_RESTORE_MODE is
# set to external. Otherwise, HC_NAME is ignored.
HC_NAME ?= ""
# HC_NAMESPACE is the namespace for HostedClusters to use for HCP tests.
HC_NAMESPACE ?= clusters

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# By default, this Makefile uses docker, as the target commands have been tested primarily with it.
# However, if docker is not available, the Makefile will attempt to use podman if it's installed.
# You may also set CONTAINER_TOOL directly as an environment variable to specify a different tool.
# If neither docker nor podman is found, or if the specified tool is unavailable, the Makefile will exit with an error.

# Set CONTAINER_TOOL to Docker or Podman if not already defined by the user
CONTAINER_TOOL ?= $(shell \
  if command -v docker >/dev/null 2>&1; then echo docker; \
  elif command -v podman >/dev/null 2>&1; then echo podman; \
  else echo ""; \
  fi \
)
ifeq ($(shell command -v $(CONTAINER_TOOL) >/dev/null 2>&1 && echo found),)
  $(error The selected container tool '$(CONTAINER_TOOL)' is not available on this system. Please install it or choose a different tool.)
endif
$(info Using Container Tool: $(CONTAINER_TOOL))

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

# Function to check tool version
# Parameters: $(1)=TOOL_NAME $(2)=TOOL_PATH $(3)=VERSION_CMD $(4)=EXPECTED_VERSION $(5)=MAKE_TARGET $(6)=DISPLAY_NAME $(7)=SPECIAL_HANDLING
define CHECK_TOOL_VERSION
	@printf "\n\033[1m$(1) Version Check:\033[0m\n"
	@if [ -f "$(2)" ]; then \
		INSTALLED_VERSION=$$($(3) || echo "unknown"); \
		EXPECTED_VERSION="$(4)"; \
		if [ "$$INSTALLED_VERSION" = "$$EXPECTED_VERSION" ]; then \
			printf "\033[32m%-30s\033[0m %-20s %s\n" "$(6)" "$$INSTALLED_VERSION" "✓ matches Makefile"; \
		$(if $(7),$(7)) \
		else \
			printf "\033[33m%-30s\033[0m %-20s %s\n" "$(6)" "$$INSTALLED_VERSION" "⚠ differs from Makefile ($$EXPECTED_VERSION)"; \
			printf "\033[33m✗ Installing the version requested by the Makefile\033[0m\n"; \
			$(MAKE) $(5); \
		fi; \
	else \
		printf "\033[31m%-30s\033[0m %-20s %s\n" "$(6)" "not found" "✗ not installed in $(LOCALBIN)"; \
		$(MAKE) $(5); \
	fi
endef

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: versions
versions: check-go ## Display all variables containing 'version' in their name.
	@printf "\033[36m%-30s\033[0m %s\n" "GO_VERSION" "$$(go version | awk '{print $$3}')"
	@printf "\n\033[1m%-30s %-20s %s\033[0m\n" "Tool and Project Versions:" "Value" "Used by Targets"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "DEFAULT_VERSION" "$(DEFAULT_VERSION)" "bundle-isupdated"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "VERSION" "$(VERSION)" "bundle, catalog-build, deploy-olm-stsflow, undeploy-olm"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "OPERATOR_SDK_VERSION" "$(OPERATOR_SDK_VERSION)" "operator-sdk"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "ENVTEST_K8S_VERSION" "$(ENVTEST_K8S_VERSION)" "test"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "GOLANGCI_LINT_VERSION" "$(GOLANGCI_LINT_VERSION)" "golangci-lint"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "KUSTOMIZE_VERSION" "$(KUSTOMIZE_VERSION)" "kustomize"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "CONTROLLER_TOOLS_VERSION" "$(CONTROLLER_TOOLS_VERSION)" "controller-gen"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "OPM_VERSION" "$(OPM_VERSION)" "opm"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "PREVIOUS_CHANNEL" "$(PREVIOUS_CHANNEL)" "catalog-test-upgrade"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "PREVIOUS_CHANNEL_GO_VERSION" "$(PREVIOUS_CHANNEL_GO_VERSION)" "catalog-test-upgrade"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "GO_TOOLCHAIN_VERSION" "$(GO_TOOLCHAIN_VERSION)" "(informational only)"
	$(call CHECK_TOOL_VERSION,Operator-SDK,$(OPERATOR_SDK),$(OPERATOR_SDK) version 2>/dev/null | grep 'operator-sdk version' | cut -d'"' -f2,$(OPERATOR_SDK_VERSION),operator-sdk,OPERATOR_SDK_LOCAL)
	$(call CHECK_TOOL_VERSION,Controller-Gen,$(CONTROLLER_GEN),$(CONTROLLER_GEN) --version 2>/dev/null | grep 'Version:' | cut -d' ' -f2,$(CONTROLLER_TOOLS_VERSION),controller-gen,CONTROLLER_GEN_LOCAL)
	$(call CHECK_TOOL_VERSION,OPM,$(OPM),$(OPM) version 2>/dev/null | cut -d'"' -f2,$(OPM_VERSION),opm,OPM_LOCAL)
	$(call CHECK_TOOL_VERSION,Golangci-Lint,$(GOLANGCI_LINT),$(GOLANGCI_LINT) --version 2>/dev/null | grep 'golangci-lint has version' | sed 's/.*version \([^ ]*\).*/\1/',$(GOLANGCI_LINT_VERSION),golangci-lint,GOLANGCI_LINT_LOCAL)
	$(call CHECK_TOOL_VERSION,Kustomize,$(KUSTOMIZE),$(KUSTOMIZE) version --short 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "(devel)",$(KUSTOMIZE_VERSION),kustomize,KUSTOMIZE_LOCAL,elif [ "$$INSTALLED_VERSION" = "(devel)" ]; then printf "\033[36m%-30s\033[0m %-20s %s\n" "KUSTOMIZE_LOCAL" "$$INSTALLED_VERSION" "ⓘ dev build (expected $(KUSTOMIZE_VERSION))";)

.PHONY: check-go
check-go: ## Check if go binary is available in PATH
	@if ! command -v go >/dev/null 2>&1; then \
		printf "\033[31m✗ Error: 'go' binary not found in PATH\033[0m\n"; \
		printf "Please install Go from https://golang.org/dl/ and ensure it's in your PATH\n"; \
		exit 1; \
	fi
	@printf "\033[32m✓ Go binary found in PATH\033[0m\n"

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: check-go ## Run go fmt against code.
	go fmt -mod=mod ./...

.PHONY: vet
vet: check-go ## Run go vet against code.
	go vet -mod=mod ./...

# If test results in prow are different, it is because the environment used.
# You can simulate their env by running
# docker run --platform linux/amd64 -w $PWD -v $PWD:$PWD -it registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.14 sh -c "make test"
# where the image corresponds to the prow config for the test job, https://github.com/openshift/release/blob/master/ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-dev.yaml#L1-L5
# to login to registry cluster follow https://docs.ci.openshift.org/docs/how-tos/use-registries-in-build-farm/#how-do-i-log-in-to-pull-images-that-require-authentication
# If bin/ contains binaries of different arch, you may remove them so the container can install their arch.
.PHONY: test
test: check-go vet envtest ## Run unit tests; run Go linters checks; check if api and bundle folders are up to date; and check if go dependencies are valid
	@make versions
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -mod=mod $(shell go list -mod=mod ./... | grep -v /tests/e2e) -coverprofile cover.out
	@make lint
	@make api-isupdated
	@make bundle-isupdated
	@make check-go-dependencies


# Lint CLI needs to be built from the same toolchain version
GOLANGCI_LINT = $(LOCALBIN)/$(BRANCH_VERSION)/golangci-lint
.PHONY: golangci-lint $(GOLANGCI_LINT)
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool-versioned,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION),$(GOLANGCI_LINT_VERSION))
	@if [ -L "$(LOCALBIN)/golangci-lint" ]; then \
		unlink "$(LOCALBIN)/golangci-lint"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/golangci-lint" "$(LOCALBIN)/golangci-lint"

.PHONY: lint
lint: golangci-lint ## Run Go linters checks against all project's Go files.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Fix Go linters issues.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: check-go manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: check-go manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

OC_CLI ?= $(shell which oc)

# makes CLUSTER_TYPE quieter when unauthenticated
CLUSTER_TYPE_SHELL := $(shell $(OC_CLI) get infrastructures cluster -o jsonpath='{.status.platform}' 2> /dev/null | tr A-Z a-z)
CLUSTER_TYPE ?= $(CLUSTER_TYPE_SHELL)
CLUSTER_OS = $(shell $(OC_CLI) get node -o jsonpath='{.items[0].status.nodeInfo.operatingSystem}' 2> /dev/null)
CLUSTER_ARCH = $(shell $(OC_CLI) get node -o jsonpath='{.items[0].status.nodeInfo.architecture}' 2> /dev/null)

# If using podman machine, and host platform is not linux/amd64 run
# - podman machine ssh sudo rpm-ostree install qemu-user-static && sudo systemctl reboot
# from: https://github.com/containers/podman/issues/12144#issuecomment-955760527
# related enhancements that may remove the need to manually install qemu-user-static https://bugzilla.redhat.com/show_bug.cgi?id=2061584
DOCKER_BUILD_ARGS ?= --platform=linux/amd64
ifneq ($(CLUSTER_TYPE),)
	DOCKER_BUILD_ARGS = --platform=$(CLUSTER_OS)/$(CLUSTER_ARCH)
endif
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build --load -t $(IMG) . $(DOCKER_BUILD_ARGS)

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/$(BRANCH_VERSION)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/$(BRANCH_VERSION)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: kustomize $(KUSTOMIZE)
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool-versioned,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION),$(KUSTOMIZE_VERSION))
	@if [ -L "$(LOCALBIN)/kustomize" ]; then \
		unlink "$(LOCALBIN)/kustomize"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/kustomize" "$(LOCALBIN)/kustomize"

.PHONY: controller-gen $(CONTROLLER_GEN)
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool-versioned,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION),$(CONTROLLER_TOOLS_VERSION))
	@if [ -L "$(LOCALBIN)/controller-gen" ]; then \
		unlink "$(LOCALBIN)/controller-gen"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/controller-gen" "$(LOCALBIN)/controller-gen"

# Uses go-install-tool-versioned (see its doc comment below) for both the version and
# architecture check, rather than a bespoke arch-only check here.
.PHONY: envtest $(ENVTEST)
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool-versioned,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20250308055145-5fe7bb3edc86,v0.0.0-20250308055145-5fe7bb3edc86)

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/$(BRANCH_VERSION)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifneq ($(shell $(OPERATOR_SDK) version | cut -d'"' -f2),$(OPERATOR_SDK_VERSION))
	set -e; \
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl --retry 5 --retry-delay 5 -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK);
endif
	@if [ -L "$(LOCALBIN)/operator-sdk" ]; then \
		unlink "$(LOCALBIN)/operator-sdk"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/operator-sdk" "$(LOCALBIN)/operator-sdk"


.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	GOFLAGS="-mod=mod" $(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && GOFLAGS="-mod=mod" $(KUSTOMIZE) edit set image controller=$(IMG)
	GOFLAGS="-mod=mod" $(KUSTOMIZE) build config/manifests | GOFLAGS="-mod=mod" $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	@make nullables
	# Copy updated bundle.Dockerfile to CI's Dockerfile.bundle
	# TODO: update CI to use generated one
	cp bundle.Dockerfile build/Dockerfile.bundle
	GOFLAGS="-mod=mod" $(OPERATOR_SDK) bundle validate ./bundle
	$(SED) -e 's/    createdAt: .*/$(shell grep -I '^    createdAt: ' bundle/manifests/oadp-operator.clusterserviceversion.yaml)/' bundle/manifests/oadp-operator.clusterserviceversion.yaml > bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp
	mv bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp bundle/manifests/oadp-operator.clusterserviceversion.yaml

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build --load -f bundle.Dockerfile -t $(BUNDLE_IMG) . $(DOCKER_BUILD_ARGS)

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM ?= $(LOCALBIN)/$(BRANCH_VERSION)/opm
opm: ## Download opm locally if necessary.
ifneq ($(shell $(OPM) version | cut -d'"' -f2),$(OPM_VERSION))
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl --retry 5 --retry-delay 5 -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM)
endif
	@if [ -L "$(LOCALBIN)/opm" ]; then \
		unlink "$(LOCALBIN)/opm"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/opm" "$(LOCALBIN)/opm"

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Catalog source name for deploy-olm
CATALOG_SOURCE_NAME ?= oadp-operator-catalog

# Build a catalog image using file-based catalog (FBC) format.
# This renders bundle images into a declarative config, adds a channel entry,
# validates the catalog, and builds a container image with proper platform support.
# The FBC approach replaces the deprecated sqlite-based 'opm index add' method
# and ensures correct multi-arch builds via $(DOCKER_BUILD_ARGS).
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	rm -rf catalog.Dockerfile catalog/
	mkdir -p catalog/oadp-operator
	$(OPM) render $(BUNDLE_IMGS) -o yaml > catalog/oadp-operator/index.yaml
	@echo "---" >> catalog/oadp-operator/index.yaml
	@echo "schema: olm.package" >> catalog/oadp-operator/index.yaml
	@echo "name: oadp-operator" >> catalog/oadp-operator/index.yaml
	@echo "defaultChannel: $(DEFAULT_CHANNEL)" >> catalog/oadp-operator/index.yaml
	@echo "---" >> catalog/oadp-operator/index.yaml
	@echo "schema: olm.channel" >> catalog/oadp-operator/index.yaml
	@echo "name: $(DEFAULT_CHANNEL)" >> catalog/oadp-operator/index.yaml
	@echo "package: oadp-operator" >> catalog/oadp-operator/index.yaml
	@echo "entries:" >> catalog/oadp-operator/index.yaml
	@echo "  - name: oadp-operator.v$(VERSION)" >> catalog/oadp-operator/index.yaml
	$(OPM) validate catalog/
	$(OPM) generate dockerfile catalog/ --binary-image=quay.io/operator-framework/opm:$(OPM_VERSION)
	$(CONTAINER_TOOL) build --load $(DOCKER_BUILD_ARGS) -f catalog.Dockerfile -t $(CATALOG_IMG) .
	rm -rf catalog.Dockerfile catalog/

# Build a catalog image using build/Dockerfile.catalog (self-contained, used by CI).
# Passes OPM_VERSION from this Makefile to keep the two in sync.
#
# Use case: test the same Dockerfile that CI uses, locally.
#   make catalog-fbc-build BUNDLE_IMG=quay.io/konveyor/oadp-operator-bundle:latest
#   make catalog-push
#
# Then install on-cluster:
#   OLMv0 (CatalogSource + Subscription):
#     make deploy-olm CATALOG_IMG=$(CATALOG_IMG)
#   OLMv1 (ClusterExtension):
#     kubectl apply -f - <<EOF
#     apiVersion: olm.operatorframework.io/v1
#     kind: ClusterExtension
#     metadata:
#       name: oadp-operator
#     spec:
#       source:
#         sourceType: Catalog
#         catalog:
#           packageName: oadp-operator
#       install:
#         namespace: openshift-adp
#         serviceAccount:
#           name: oadp-operator-controller-manager
#     EOF
.PHONY: catalog-fbc-build
catalog-fbc-build: ## Build a catalog image from build/Dockerfile.catalog.
	$(CONTAINER_TOOL) build --load $(DOCKER_BUILD_ARGS) \
		-f build/Dockerfile.catalog \
		--build-arg BUNDLE_IMG=$(BUNDLE_IMG) \
		--build-arg OPM_VERSION=$(OPM_VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg DEFAULT_CHANNEL=$(DEFAULT_CHANNEL) \
		-t $(CATALOG_IMG) .

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

##@ oadp specifics

.PHONY: api-isupdated
api-isupdated: TEMP:= $(shell mktemp -d)
api-isupdated:
	@cp -r ./ $(TEMP) && cd $(TEMP) && make generate && cd - && diff -ruN api/ $(TEMP)/api/ && echo "api is up to date" || (echo "api is out of date, run 'make generate' to update" && exit 1)
	@chmod -R 777 $(TEMP) && rm -rf $(TEMP)

.PHONY: bundle-isupdated
bundle-isupdated: TEMP:= $(shell mktemp -d)
bundle-isupdated: VERSION:= $(DEFAULT_VERSION) #prevent VERSION overrides from https://github.com/openshift/release/blob/f1a388ab05d493b6d95b8908e28687b4c0679498/clusters/build-clusters/01_cluster/ci/_origin-release-build/golang-1.19/Dockerfile#LL9C1-L9C1
bundle-isupdated:
	@cp -r ./ $(TEMP) && cd $(TEMP) && make bundle && cd - && diff -ruN bundle/ $(TEMP)/bundle/ && echo "bundle is up to date" || (echo "bundle is out of date, run 'make bundle' to update" && exit 1)
	@chmod -R 777 $(TEMP) && rm -rf $(TEMP)

.PHONY: check-go-dependencies
check-go-dependencies: TEMP:= $(shell mktemp -d)
check-go-dependencies: check-go
	@cp -r ./ $(TEMP) && cd $(TEMP) && go mod tidy && cd - && diff -ruN ./ $(TEMP)/ && echo "go dependencies checked" || (echo "go dependencies are out of date, run 'go mod tidy' to update" && exit 1)
	@chmod -R 777 $(TEMP) && rm -rf $(TEMP)
	go mod verify

SED = sed
# if on macos, install gsed
# https://formulae.brew.sh/formula/gnu-sed

# Codecov OS String for use in download url
ifeq ($(OS),Windows_NT)
    OS_String = windows
else
    UNAME_S := $(shell uname -s)
    ifeq ($(UNAME_S),Linux)
        OS_String = linux
    endif
    ifeq ($(UNAME_S),Darwin)
        OS_String = macos
		SED = gsed
    endif
endif
submit-coverage:
	curl --retry 5 --retry-delay 5 -Os https://uploader.codecov.io/latest/$(OS_String)/codecov
	chmod +x codecov
	./codecov -C $(shell git rev-parse HEAD) -r openshift/oadp-operator --nonZero
	rm -f codecov

# go-install-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool
[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install -mod=mod $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# go-install-tool-branch will 'go install' any package $2 and install it to branch-specific directory $1.
define go-install-tool-branch
[ -f $(1) ] || { \
set -e ;\
mkdir -p $(dir $(1)) ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2) to branch directory" ;\
GOBIN=$(dir $(1)) go install -a -mod=mod $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# go-install-tool-versioned installs $2 to branch-specific path $1, but only if $1 is missing,
# doesn't have the pinned module version $3 embedded in it, or was built for a different
# GOOS/GOARCH than this host. Uses `go version -m` to read a binary's embedded build info
# (module version, GOOS, GOARCH) instead of executing it or trusting a sidecar marker file:
#   - A binary's own --version/--help output is not a reliable version or health signal. It
#     can depend on ldflags a tool's own release process sets, which `go install` doesn't set
#     (e.g. kustomize can report "(devel)" or an unexpanded `$$Format:%H$$` placeholder instead
#     of its real version), and some tools exit nonzero on --version even when perfectly
#     healthy (kustomize exits 1, setup-envtest's --help exits 2) — so "nonzero exit means
#     broken" is the wrong signal. It's also dangerous under this Makefile's
#     `.SHELLFLAGS = -ec` (enables `set -e`, honored by GNU Make 3.82+ but silently ignored by
#     the Make 3.81 macOS ships): a bare probe command that exits nonzero aborts the whole
#     recipe unless it's wrapped in an `if`/`||` guard.
#   - Executing the binary to test compatibility can't detect a wrong-arch binary at all
#     inside a container with qemu-user-static/binfmt_misc registered (common in multi-arch
#     CI/build images): the foreign-arch binary runs under emulation and returns its own exit
#     code rather than an exec-format-error.
# Reading embedded build info sidesteps both: no execution means no exit-code heuristic to
# get wrong and no qemu blind spot.
define go-install-tool-versioned
@BUILDINFO="$$(go version -m $(1) 2>/dev/null)" || BUILDINFO="" ;\
MOD_VERSION="$$(printf '%s\n' "$$BUILDINFO" | awk '$$1=="mod"{print $$3; exit}')" ;\
if [ -n "$$BUILDINFO" ] && [ "$$MOD_VERSION" = "$(3)" ] && printf '%s\n' "$$BUILDINFO" | grep -qF "GOOS=$$(go env GOOS)" && printf '%s\n' "$$BUILDINFO" | grep -qF "GOARCH=$$(go env GOARCH)"; then \
	echo "$(notdir $(1)) $(3) is already installed" ;\
else \
	set -e ;\
	mkdir -p $(dir $(1)) ;\
	rm -f $(1) ;\
	TMP_DIR=$$(mktemp -d) ;\
	cd $$TMP_DIR ;\
	go mod init tmp ;\
	echo "Installing $(notdir $(1)) $(3)" ;\
	GOBIN=$(dir $(1)) go install -a -mod=mod $(2) ;\
	cd - >/dev/null ;\
	rm -rf $$TMP_DIR ;\
fi
endef

YQ = $(LOCALBIN)/yq
yq: ## Download yq locally if necessary.
	# 4.28.1 is latest with go 1.17 go.mod
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4@v4.28.1)

.PHONY: nullables
nullables: ## patch nullables in CRDs
	@make nullable-crds-bundle nullable-crds-config

.PHONY: nullable-crds-bundle
nullable-crds-bundle: DPA_SPEC_CONFIG_PROP = .spec.versions.0.schema.openAPIV3Schema.properties.spec.properties.configuration.properties
nullable-crds-bundle: PROP_RESOURCE_ALLOC = properties.podConfig.properties.resourceAllocations
nullable-crds-bundle: VELERO_RESOURCE_ALLOC = $(DPA_SPEC_CONFIG_PROP).velero.$(PROP_RESOURCE_ALLOC)
nullable-crds-bundle: RESTIC_RESOURCE_ALLOC = $(DPA_SPEC_CONFIG_PROP).restic.$(PROP_RESOURCE_ALLOC)
nullable-crds-bundle: DPA_CRD_YAML ?= bundle/manifests/oadp.openshift.io_dataprotectionapplications.yaml
nullable-crds-bundle: yq
# Velero CRD
	@$(YQ) '$(VELERO_RESOURCE_ALLOC).nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(VELERO_RESOURCE_ALLOC).properties.limits.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(VELERO_RESOURCE_ALLOC).properties.limits.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(VELERO_RESOURCE_ALLOC).properties.requests.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(VELERO_RESOURCE_ALLOC).properties.requests.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
# Restic CRD
	@$(YQ) '$(RESTIC_RESOURCE_ALLOC).nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(RESTIC_RESOURCE_ALLOC).properties.limits.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(RESTIC_RESOURCE_ALLOC).properties.limits.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(RESTIC_RESOURCE_ALLOC).properties.requests.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(YQ) '$(RESTIC_RESOURCE_ALLOC).properties.requests.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)

.PHONY: nullable-crds-config
nullable-crds-config: DPA_CRD_YAML ?= config/crd/bases/oadp.openshift.io_dataprotectionapplications.yaml
nullable-crds-config:
	@ DPA_CRD_YAML=$(DPA_CRD_YAML) make nullable-crds-bundle


.PHONY: login-required
login-required:
ifeq ($(CLUSTER_TYPE),)
	$(error You must be logged in to a cluster to run this command)
else
	$(info $$CLUSTER_TYPE is [${CLUSTER_TYPE}])
endif

GIT_REV:=$(shell git rev-parse --short HEAD)

# Namespace to deploy OADP operator, used by Makefile commands
OADP_TEST_NAMESPACE ?= openshift-adp
# Namespace to install CatalogSource (openshift-marketplace is the standard location)
CATALOG_SOURCE_NAMESPACE ?= openshift-marketplace

.PHONY: deploy-olm
deploy-olm: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):$(TTL_DURATION) # Set target specific variable
deploy-olm: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):$(TTL_DURATION) # Set target specific variable
deploy-olm: THIS_CATALOG_IMAGE?=ttl.sh/oadp-operator-catalog-$(GIT_REV):$(TTL_DURATION) # Set target specific variable
deploy-olm: DEPLOY_TMP:=$(shell mktemp -d)/ # Set target specific variable
deploy-olm: undeploy-olm ## Build current branch operator image, bundle image, push and install via OLM. For more information, check docs/developer/install_from_source.md
	@make versions
	@echo "DEPLOY_TMP: $(DEPLOY_TMP)"
	# build and push operator, bundle, and catalog images
	cp -r . $(DEPLOY_TMP) && cd $(DEPLOY_TMP) && \
	IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) BUNDLE_IMGS=$(THIS_BUNDLE_IMAGE) CATALOG_IMG=$(THIS_CATALOG_IMAGE) \
		make docker-build docker-push bundle bundle-build bundle-push catalog-build catalog-push; \
	chmod -R 777 $(DEPLOY_TMP) && rm -rf $(DEPLOY_TMP)
	# Create CatalogSource with restricted security context
	@echo "Creating CatalogSource $(CATALOG_SOURCE_NAME)..."
	@echo -e "apiVersion: operators.coreos.com/v1alpha1\nkind: CatalogSource\nmetadata:\n  name: $(CATALOG_SOURCE_NAME)\n  namespace: $(CATALOG_SOURCE_NAMESPACE)\nspec:\n  sourceType: grpc\n  image: $(THIS_CATALOG_IMAGE)\n  displayName: OADP Operator Catalog\n  publisher: OADP Team\n  grpcPodConfig:\n    securityContextConfig: restricted" | $(OC_CLI) apply -f -
	# Wait for CatalogSource to be ready
	@echo "Waiting for CatalogSource to be ready..."
	@timeout=120; \
	while [ $$timeout -gt 0 ]; do \
		STATE=$$($(OC_CLI) get catalogsource $(CATALOG_SOURCE_NAME) -n $(CATALOG_SOURCE_NAMESPACE) -o jsonpath='{.status.connectionState.lastObservedState}' 2>/dev/null); \
		if [ "$$STATE" = "READY" ]; then \
			echo "CatalogSource is ready"; \
			break; \
		fi; \
		echo -n "."; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Timeout waiting for CatalogSource"; \
		echo "=== CatalogSource status ==="; \
		$(OC_CLI) get catalogsource $(CATALOG_SOURCE_NAME) -n $(CATALOG_SOURCE_NAMESPACE) -o yaml; \
		echo "=== Catalog pod status ==="; \
		$(OC_CLI) get pods -n $(CATALOG_SOURCE_NAMESPACE) -l olm.catalogSource=$(CATALOG_SOURCE_NAME) 2>/dev/null || true; \
		echo "=== Catalog pod logs ==="; \
		$(OC_CLI) logs -n $(CATALOG_SOURCE_NAMESPACE) -l olm.catalogSource=$(CATALOG_SOURCE_NAME) --tail=50 2>/dev/null || true; \
		exit 1; \
	fi
	# Create OperatorGroup if not exists
	@echo "Checking OperatorGroup..."
	@OG_COUNT=$$($(OC_CLI) get operatorgroup -n $(OADP_TEST_NAMESPACE) --no-headers 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$OG_COUNT" -eq "0" ]; then \
		echo "Creating OperatorGroup..."; \
		echo -e "apiVersion: operators.coreos.com/v1\nkind: OperatorGroup\nmetadata:\n  name: oadp-operator-group\n  namespace: $(OADP_TEST_NAMESPACE)\nspec:\n  targetNamespaces:\n    - $(OADP_TEST_NAMESPACE)" | $(OC_CLI) apply -f -; \
	else \
		echo "OperatorGroup already exists"; \
	fi
	# Create Subscription
	@echo "Creating Subscription..."
	@echo -e "apiVersion: operators.coreos.com/v1alpha1\nkind: Subscription\nmetadata:\n  name: oadp-operator\n  namespace: $(OADP_TEST_NAMESPACE)\nspec:\n  channel: $(DEFAULT_CHANNEL)\n  name: oadp-operator\n  source: $(CATALOG_SOURCE_NAME)\n  sourceNamespace: $(CATALOG_SOURCE_NAMESPACE)\n  installPlanApproval: Automatic" | $(OC_CLI) apply -f -
	# Wait for operator to be ready
	@echo "Waiting for InstallPlan to be created..."
	@timeout=60; \
	while [ $$timeout -gt 0 ]; do \
		INSTALL_PLAN=$$($(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.status.installPlanRef.name}' 2>/dev/null); \
		if [ -n "$$INSTALL_PLAN" ]; then \
			echo "InstallPlan $$INSTALL_PLAN found"; \
			break; \
		fi; \
		echo -n "."; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Timeout waiting for InstallPlan"; \
		exit 1; \
	fi
	@echo "Waiting for CSV to exist..."
	@timeout=120; \
	CSV_NAME=""; \
	while [ $$timeout -gt 0 ]; do \
		CSV_NAME=$$($(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.status.currentCSV}' 2>/dev/null); \
		if [ -n "$$CSV_NAME" ]; then \
			if $(OC_CLI) get csv/$$CSV_NAME -n $(OADP_TEST_NAMESPACE) >/dev/null 2>&1; then \
				echo "CSV $$CSV_NAME found"; \
				break; \
			fi; \
		fi; \
		echo -n "."; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Timeout waiting for CSV to exist"; \
		exit 1; \
	fi
	@echo "Waiting for CSV to be ready..."
	@CSV_NAME=$$($(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.status.currentCSV}' 2>/dev/null); \
	if [ -n "$$CSV_NAME" ]; then \
		$(OC_CLI) wait --for=jsonpath='{.status.phase}'=Succeeded csv/$$CSV_NAME -n $(OADP_TEST_NAMESPACE) --timeout=300s; \
	fi
	@echo "Operator is ready!"
	@$(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE)
	@$(OC_CLI) get csv -n $(OADP_TEST_NAMESPACE)

.PHONY: undeploy-olm
undeploy-olm: login-required ## Uninstall current branch operator via OLM
	$(OC_CLI) whoami # Check if logged in
	$(OC_CLI) create namespace $(OADP_TEST_NAMESPACE) || true
	# Delete Subscription
	-$(OC_CLI) delete subscription oadp-operator -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true
	# Delete any subscriptions using our catalog
	-$(OC_CLI) get subscription -n $(OADP_TEST_NAMESPACE) -o name 2>/dev/null | xargs -I {} $(OC_CLI) get {} -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.metadata.name}{"\t"}{.spec.source}{"\n"}' 2>/dev/null | grep "$(CATALOG_SOURCE_NAME)" | cut -f1 | xargs -I {} $(OC_CLI) delete subscription {} -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true || true
	# Delete CSV with OADP label
	-$(OC_CLI) delete csv -l operators.coreos.com/oadp-operator.$(OADP_TEST_NAMESPACE) -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true
	# Delete any CSV starting with oadp-operator
	-$(OC_CLI) get csv -n $(OADP_TEST_NAMESPACE) -o name 2>/dev/null | grep oadp-operator | xargs -I {} $(OC_CLI) delete {} -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true || true
	# Delete CatalogSource
	-$(OC_CLI) delete catalogsource $(CATALOG_SOURCE_NAME) -n $(CATALOG_SOURCE_NAMESPACE) --ignore-not-found=true
	# Delete OperatorGroup (only if we created it)
	-$(OC_CLI) delete operatorgroup oadp-operator-group -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true

# Create subscription YAML helper function
# Parameters:
#   $(1) - Path to the subscription YAML file to create (e.g., /tmp/oadp-gcp-subscription.yaml)
create-sts-subscription = \
	echo "apiVersion: operators.coreos.com/v1alpha1" > $(1) && \
	echo "kind: Subscription" >> $(1) && \
	echo "metadata:" >> $(1) && \
	echo "  name: oadp-operator" >> $(1) && \
	echo "  namespace: $(OADP_TEST_NAMESPACE)" >> $(1) && \
	echo "spec:" >> $(1) && \
	echo "  channel: $(DEFAULT_CHANNEL)" >> $(1) && \
	echo "  name: oadp-operator" >> $(1) && \
	echo "  source: $(CATALOG_SOURCE_NAME)" >> $(1) && \
	echo "  sourceNamespace: $(CATALOG_SOURCE_NAMESPACE)" >> $(1) && \
	echo "  installPlanApproval: Automatic" >> $(1) && \
	echo "  config:" >> $(1) && \
	echo "    env:" >> $(1)

# Apply subscription and wait for ready helper function
# Parameters:
#   $(1) - Path to the subscription YAML file to apply (e.g., /tmp/oadp-gcp-subscription.yaml)
#   $(2) - Cloud provider descriptive name for status messages (e.g., "GCP WIF", "AWS STS", "Azure Workload Identity")
apply-sts-subscription = \
	$(OC_CLI) apply -f $(1) && \
	rm -f $(1) && \
	echo "" && \
	echo "Subscription created with $(2) environment variables." && \
	echo "Waiting for operator to be ready..." && \
	echo "Waiting for InstallPlan to be created..." && \
	timeout=60; \
	while [ $$timeout -gt 0 ]; do \
		INSTALL_PLAN=$$($(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.status.installPlanRef.name}' 2>/dev/null); \
		if [ -n "$$INSTALL_PLAN" ]; then \
			echo "InstallPlan $$INSTALL_PLAN found"; \
			break; \
		fi; \
		echo -n "."; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Timeout waiting for InstallPlan"; \
		exit 1; \
	fi; \
	echo "Waiting for CSV to exist..."; \
	timeout=120; \
	while [ $$timeout -gt 0 ]; do \
		CSV_NAME=$$($(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.status.currentCSV}' 2>/dev/null); \
		if [ -n "$$CSV_NAME" ]; then \
			if $(OC_CLI) get csv/$$CSV_NAME -n $(OADP_TEST_NAMESPACE) >/dev/null 2>&1; then \
				echo "CSV $$CSV_NAME found"; \
				break; \
			fi; \
		fi; \
		echo -n "."; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Timeout waiting for CSV to exist"; \
		exit 1; \
	fi; \
	echo "Waiting for CSV to be ready..."; \
	if [ -n "$$CSV_NAME" ]; then \
		$(OC_CLI) wait --for=jsonpath='{.status.phase}'=Succeeded csv/$$CSV_NAME -n $(OADP_TEST_NAMESPACE) --timeout=300s; \
	fi; \
	echo "Operator is ready!"; \
	$(OC_CLI) get subscription oadp-operator -n $(OADP_TEST_NAMESPACE); \
	$(OC_CLI) get csv -n $(OADP_TEST_NAMESPACE)

.PHONY: deploy-olm-stsflow
deploy-olm-stsflow: deploy-olm ## Deploy via OLM then uninstall CSV/Subscription and provide console URL for standardized flow
	@echo "Uninstalling CSV and Subscription to trigger standardized flow UI..."
	-$(OC_CLI) get subscription -n $(OADP_TEST_NAMESPACE) -o name 2>/dev/null | xargs -I {} $(OC_CLI) get {} -n $(OADP_TEST_NAMESPACE) -o jsonpath='{.metadata.name}{"\t"}{.spec.source}{"\n"}' 2>/dev/null | grep "$(CATALOG_SOURCE_NAME)" | cut -f1 | xargs -I {} $(OC_CLI) delete subscription {} -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true || true
	-$(OC_CLI) delete csv oadp-operator.v$(VERSION) -n $(OADP_TEST_NAMESPACE) --ignore-not-found=true
	@echo ""
	@echo "==========================================================================="
	@echo "Open the following URL in your browser to trigger the standardized flow UI:"
	@echo ""
	@CONSOLE_URL=$$($(OC_CLI) get route console -n openshift-console -o jsonpath='{.spec.host}'); \
	echo "https://$$CONSOLE_URL/operatorhub/ns/$(OADP_TEST_NAMESPACE)?keyword=oadp-operator&details-item=oadp-operator-$(CATALOG_SOURCE_NAME)-$(OADP_TEST_NAMESPACE)&channel=$(DEFAULT_CHANNEL)&version=$(VERSION)"
	@echo ""
	@echo "==========================================================================="

.PHONY: deploy-olm-stsflow-gcp
deploy-olm-stsflow-gcp: deploy-olm-stsflow ## Deploy via OLM with GCP WIF standardized flow and create subscription with GCP env vars
	@if [ -n "$(GCP_PROJECT_NUM)" ] && [ -n "$(GCP_POOL_ID)" ] && [ -n "$(GCP_PROVIDER_ID)" ] && [ -n "$(GCP_SA_EMAIL)" ]; then \
		echo "Creating subscription with GCP WIF environment variables..."; \
		$(call create-sts-subscription,/tmp/oadp-gcp-subscription.yaml); \
		echo "    - name: PROJECT_NUMBER" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "      value: \"$(GCP_PROJECT_NUM)\"" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "    - name: POOL_ID" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "      value: \"$(GCP_POOL_ID)\"" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "    - name: PROVIDER_ID" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "      value: \"$(GCP_PROVIDER_ID)\"" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "    - name: SERVICE_ACCOUNT_EMAIL" >> /tmp/oadp-gcp-subscription.yaml; \
		echo "      value: \"$(GCP_SA_EMAIL)\"" >> /tmp/oadp-gcp-subscription.yaml; \
		$(call apply-sts-subscription,/tmp/oadp-gcp-subscription.yaml,GCP WIF); \
	else \
		echo ""; \
		echo "GCP WIF environment variables not set. Please set all of the following:"; \
		echo "  GCP_PROJECT_NUM"; \
		echo "  GCP_POOL_ID"; \
		echo "  GCP_PROVIDER_ID"; \
		echo "  GCP_SA_EMAIL"; \
		echo ""; \
		echo "Example:"; \
		echo "  make deploy-olm-stsflow-gcp GCP_PROJECT_NUM=123456789 GCP_POOL_ID=my-pool GCP_PROVIDER_ID=my-provider GCP_SA_EMAIL=my-sa@my-project.iam.gserviceaccount.com"; \
	fi

.PHONY: deploy-olm-stsflow-aws
deploy-olm-stsflow-aws: deploy-olm-stsflow ## Deploy via OLM with AWS STS standardized flow and create subscription with AWS env vars
	@if [ -n "$(AWS_ROLE_ARN)" ]; then \
		echo "Creating subscription with AWS STS environment variables..."; \
		$(call create-sts-subscription,/tmp/oadp-aws-subscription.yaml); \
		echo "    - name: ROLEARN" >> /tmp/oadp-aws-subscription.yaml; \
		echo "      value: \"$(AWS_ROLE_ARN)\"" >> /tmp/oadp-aws-subscription.yaml; \
		$(call apply-sts-subscription,/tmp/oadp-aws-subscription.yaml,AWS STS); \
	else \
		echo ""; \
		echo "AWS STS environment variable not set. Please set:"; \
		echo "  AWS_ROLE_ARN"; \
		echo ""; \
		echo "Example:"; \
		echo "  make deploy-olm-stsflow-aws AWS_ROLE_ARN=arn:aws:iam::123456789012:role/my-oadp-role"; \
	fi

.PHONY: deploy-olm-stsflow-azure
deploy-olm-stsflow-azure: deploy-olm-stsflow ## Deploy via OLM with Azure Workload Identity standardized flow and create subscription with Azure env vars
	@if [ -n "$(AZURE_CLIENT_ID)" ] && [ -n "$(AZURE_TENANT_ID)" ] && [ -n "$(AZURE_SUBSCRIPTION_ID)" ]; then \
		echo "Creating subscription with Azure Workload Identity environment variables..."; \
		$(call create-sts-subscription,/tmp/oadp-azure-subscription.yaml); \
		echo "    - name: CLIENTID" >> /tmp/oadp-azure-subscription.yaml; \
		echo "      value: \"$(AZURE_CLIENT_ID)\"" >> /tmp/oadp-azure-subscription.yaml; \
		echo "    - name: TENANTID" >> /tmp/oadp-azure-subscription.yaml; \
		echo "      value: \"$(AZURE_TENANT_ID)\"" >> /tmp/oadp-azure-subscription.yaml; \
		echo "    - name: SUBSCRIPTIONID" >> /tmp/oadp-azure-subscription.yaml; \
		echo "      value: \"$(AZURE_SUBSCRIPTION_ID)\"" >> /tmp/oadp-azure-subscription.yaml; \
		$(call apply-sts-subscription,/tmp/oadp-azure-subscription.yaml,Azure Workload Identity); \
	else \
		echo ""; \
		echo "Azure Workload Identity environment variables not set. Please set all of the following:"; \
		echo "  AZURE_CLIENT_ID"; \
		echo "  AZURE_TENANT_ID"; \
		echo "  AZURE_SUBSCRIPTION_ID"; \
		echo ""; \
		echo "Example:"; \
		echo "  make deploy-olm-stsflow-azure AZURE_CLIENT_ID=12345678-1234-1234-1234-123456789012 AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321 AZURE_SUBSCRIPTION_ID=abcdef12-3456-7890-abcd-ef1234567890"; \
	fi

.PHONY: catalog-test-upgrade
catalog-test-upgrade: PREVIOUS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-previous-$(GIT_REV):$(TTL_DURATION)
catalog-test-upgrade: PREVIOUS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-previous-bundle-$(GIT_REV):$(TTL_DURATION)
catalog-test-upgrade: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):$(TTL_DURATION)
catalog-test-upgrade: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):$(TTL_DURATION)
catalog-test-upgrade: CATALOG_IMAGE?=ttl.sh/oadp-operator-catalog-$(GIT_REV):$(TTL_DURATION)
catalog-test-upgrade: opm login-required ## Prepare a catalog image with two channels: PREVIOUS_CHANNEL and from current branch. For more information, check docs/developer/testing/test_oadp_version_upgrade.md
	mkdir test-upgrade && rsync -a --exclude=test-upgrade ./ test-upgrade/current
	git clone --depth=1 git@github.com:openshift/oadp-operator.git -b $(PREVIOUS_CHANNEL) test-upgrade/$(PREVIOUS_CHANNEL)
	cd test-upgrade/$(PREVIOUS_CHANNEL) && \
		echo -e "FROM golang:$(PREVIOUS_CHANNEL_GO_VERSION)\nRUN useradd --create-home dev\nUSER dev\nWORKDIR /home/dev/$(PREVIOUS_CHANNEL)" | $(CONTAINER_TOOL) image build --tag catalog-test-upgrade - && \
		$(CONTAINER_TOOL) container run -u $(shell id -u):$(shell id -g) -v $(shell pwd)/test-upgrade/$(PREVIOUS_CHANNEL):/home/dev/$(PREVIOUS_CHANNEL) --rm catalog-test-upgrade make bundle IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) && \
		$(SED)  -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	cd test-upgrade/current && IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) make bundle && \
		$(SED) -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --bundles $(PREVIOUS_BUNDLE_IMAGE),$(THIS_BUNDLE_IMAGE) --tag $(CATALOG_IMAGE)
	$(CONTAINER_TOOL) push $(CATALOG_IMAGE)
	echo -e "apiVersion: operators.coreos.com/v1alpha1\nkind: CatalogSource\nmetadata:\n  name: oadp-operator-catalog-test-upgrade\n  namespace: openshift-marketplace\nspec:\n  sourceType: grpc\n  image: $(CATALOG_IMAGE)" | $(OC_CLI) create -f -
	chmod -R 777 test-upgrade && rm -rf test-upgrade && $(CONTAINER_TOOL) image rm catalog-test-upgrade

.PHONY: install-ginkgo
install-ginkgo: check-go ## Make sure ginkgo is in $GOPATH/bin
	go install -v -mod=mod github.com/onsi/ginkgo/v2/ginkgo

# CONFIGS FOR CLOUD
# bsl / blob storage cred dir
OADP_CRED_DIR ?= /var/run/oadp-credentials
# vsl / volume/cluster cred dir
CLUSTER_PROFILE_DIR ?= /Users/drajds/.aws

# bsl cred file
OADP_CRED_FILE ?= ${OADP_CRED_DIR}/new-aws-credentials
# vsl cred file
CI_CRED_FILE ?= ${CLUSTER_PROFILE_DIR}/.awscred

# aws configs - default
BSL_REGION ?= us-east-1
VSL_REGION ?= ${LEASED_RESOURCE}
BSL_AWS_PROFILE ?= default
VSL_AWS_PROFILE ?= default
# BSL_AWS_PROFILE ?= migration-engineering

# bucket file
OADP_BUCKET_FILE ?= ${OADP_CRED_DIR}/new-velero-bucket-name
# azure cluster resource file - only in CI
AZURE_RESOURCE_FILE ?= /var/run/secrets/ci.openshift.io/multi-stage/metadata.json
AZURE_CI_JSON_CRED_FILE ?= ${CLUSTER_PROFILE_DIR}/osServicePrincipal.json
AZURE_OADP_JSON_CRED_FILE ?= ${OADP_CRED_DIR}/azure-credentials

ifeq ($(CLUSTER_TYPE), gcp)
	CI_CRED_FILE = ${CLUSTER_PROFILE_DIR}/gce.json
	OADP_CRED_FILE = ${OADP_CRED_DIR}/gcp-credentials
	OADP_BUCKET_FILE = ${OADP_CRED_DIR}/gcp-velero-bucket-name
endif

ifeq ($(CLUSTER_TYPE), azure4)
	CLUSTER_TYPE = azure
endif

ifeq ($(CLUSTER_TYPE), azure)
	CI_CRED_FILE = /tmp/ci-azure-credentials
	OADP_CRED_FILE = /tmp/oadp-azure-credentials
	OADP_BUCKET_FILE = ${OADP_CRED_DIR}/azure-velero-bucket-name
endif

VELERO_PLUGIN ?= ${CLUSTER_TYPE}

ifeq ($(CLUSTER_TYPE), ibmcloud)
	VELERO_PLUGIN = aws
endif

KVM_EMULATION ?= true

ifeq ($(CLUSTER_TYPE), openstack)
	KVM_EMULATION = false
endif

OPENSHIFT_CI ?= false
OADP_BUCKET ?= $(shell cat $(OADP_BUCKET_FILE))
SETTINGS_TMP=/tmp/test-settings

.PHONY: test-e2e-setup
test-e2e-setup: login-required
	mkdir -p $(SETTINGS_TMP)
	TMP_DIR=$(SETTINGS_TMP) \
	OPENSHIFT_CI="$(OPENSHIFT_CI)" \
	PROVIDER="$(VELERO_PLUGIN)" \
	AZURE_RESOURCE_FILE="$(AZURE_RESOURCE_FILE)" \
	CI_JSON_CRED_FILE="$(AZURE_CI_JSON_CRED_FILE)" \
	OADP_JSON_CRED_FILE="$(AZURE_OADP_JSON_CRED_FILE)" \
	OADP_CRED_FILE="$(OADP_CRED_FILE)" \
	BUCKET="$(OADP_BUCKET)" \
	TARGET_CI_CRED_FILE="$(CI_CRED_FILE)" \
	VSL_AWS_PROFILE="$(VSL_AWS_PROFILE)" \
	VSL_REGION="$(VSL_REGION)" \
	BSL_REGION="$(BSL_REGION)" \
	BSL_AWS_PROFILE="$(BSL_AWS_PROFILE)" \
        SKIP_MUST_GATHER="$(SKIP_MUST_GATHER)" \
	/bin/bash "tests/e2e/scripts/$(CLUSTER_TYPE)_settings.sh"

VELERO_INSTANCE_NAME ?= velero-test
ARTIFACT_DIR ?= /tmp
# virt
HCO_UPSTREAM ?= false
TEST_VIRT_GA ?= false
TEST_VIRT ?= false
# TEST_VIRT_KDM runs only the kubevirt-datamover-specific specs (ginkgo label
# "kdm", a subset of "virt") -- for CI jobs that build/test against the
# kubevirt-datamover-controller/-plugin repos specifically and don't need the
# full TEST_VIRT suite's runtime. Tri-state, distinguished via $(origin):
#   unset          -> TEST_VIRT=true includes kdm specs too (legacy behavior,
#                      what existing openshift/release jobs rely on today)
#   explicit false -> TEST_VIRT=true excludes kdm specs, for a split non-kdm
#                      CI job
#   explicit true  -> kdm specs only, regardless of TEST_VIRT/TEST_VIRT_GA
# This lets a new split kdm-only job opt in (TEST_VIRT_KDM=true) and a new
# split non-kdm job opt out (TEST_VIRT_KDM=false) without changing what
# existing jobs that don't set this var at all get today -- see
# https://github.com/openshift/oadp-operator/issues/2413 option B.
TEST_VIRT_KDM_ORIGIN := $(origin TEST_VIRT_KDM)
TEST_VIRT_KDM ?= false
# Defaults to the upstream kubevirt/hyperconverged-cluster-index "nightly"
# moving tag rather than a pinned release, so every virt/kdm e2e run picks up
# kubevirt/kubevirt fixes (e.g. kubevirt/kubevirt#18949) automatically as soon
# as they land in a nightly build, with no version bump needed on our side.
# The OLM channel nightly publishes (e.g. "candidate-v1.20") is discovered
# live from the catalog's PackageManifest, not guessed from this tag's shape,
# so unpinned tags like "nightly" work correctly here. Override to a pinned
# release (e.g. "1.18.0") for a reproducible/stable run instead.
HCO_INDEX_TAG ?= nightly
# hcp
TEST_HCP ?= false
TEST_HCP_EXTERNAL ?= false
HCP_EXTERNAL_ARGS ?= ""
# other
TEST_CLI ?= false
SKIP_MUST_GATHER   ?= false
MUST_GATHER_REPO   ?=
MUST_GATHER_BRANCH ?= oadp-dev
ifneq ($(MUST_GATHER_REPO),)
MUST_GATHER_IMAGE  ?= ttl.sh/oadp-must-gather-$(MUST_GATHER_BRANCH)-$(GIT_REV):$(TTL_DURATION)
else
MUST_GATHER_IMAGE  ?= quay.io/konveyor/oadp-must-gather:latest
endif
TEST_UPGRADE ?= false
FAIL_FAST ?= true
TEST_FILTER = (($(shell echo '! aws && ! gcp && ! azure && ! ibmcloud' | \
$(SED) -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")) || $(CLUSTER_TYPE))
#TEST_FILTER := $(shell echo '! aws && ! gcp && ! azure' | $(SED) -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")
# TEST_VIRT_KDM=true takes precedence: it isolates the kdm job regardless of
# TEST_VIRT/TEST_VIRT_GA. Otherwise TEST_VIRT includes kdm specs unless
# TEST_VIRT_KDM was explicitly set to false (see TEST_VIRT_KDM_ORIGIN above).
ifeq ($(TEST_VIRT_KDM),true)
	TEST_FILTER += && (kdm)
else ifeq ($(TEST_VIRT),true)
	ifeq ($(TEST_VIRT_KDM_ORIGIN),undefined)
		TEST_FILTER += && (virt)
	else
		TEST_FILTER += && (virt) && (! kdm)
	endif
else ifeq ($(TEST_VIRT_GA),true)
	TEST_FILTER += && (virt)
else
	TEST_FILTER += && (! virt)
endif
# kdm specs need the same community-HCO/KubeVirt setup as the rest of the virt
# suite (TEST_VIRT's own -hco_community wiring below) -- without this,
# TEST_VIRT_KDM=true alone (i.e. without TEST_VIRT=true) would leave
# -hco_community=false and skip installing HCO entirely, breaking the kdm-only
# run before any spec even gets a VM to test against.
ifeq ($(TEST_VIRT_KDM),true)
HCO_COMMUNITY := true
else
HCO_COMMUNITY := $(TEST_VIRT)
endif
ifeq ($(TEST_UPGRADE),true)
	TEST_FILTER += && (upgrade)
else
	TEST_FILTER += && (! upgrade)
endif
ifeq ($(TEST_HCP),true)
	TEST_FILTER += && (hcp)
else
	TEST_FILTER += && (! hcp)
endif
ifeq ($(TEST_HCP_EXTERNAL),true)
	TEST_FILTER += && (hcp_external)
	HCP_EXTERNAL_ARGS = -hc_backup_restore_mode=$(HC_BACKUP_RESTORE_MODE) -hc_name=$(HC_NAME) -hc_namespace=$(HC_NAMESPACE) -sc_kubeconfig=$(SC_KUBECONFIG)
else
	TEST_FILTER += && (! hcp_external)
endif
ifeq ($(TEST_CLI),true)
	TEST_FILTER += && (cli)
else
	TEST_FILTER += && (! cli)
endif
# Do not fail fast in OpenShift CI, it's expensive to start the cluster, run all tests and report the results.
ifeq ($(OPENSHIFT_CI),true)
	FAIL_FAST = false
endif
GINKGO_FLAGS = --vv \
	--no-color=$(OPENSHIFT_CI) \
	--label-filter="$(TEST_FILTER)" \
	--junit-report="$(ARTIFACT_DIR)/junit_report.xml" \
	--fail-fast=$(FAIL_FAST) \
	--timeout=2h

.PHONY: build-must-gather
build-must-gather: ## Build must-gather image from GitHub source. Requires MUST_GATHER_REPO (e.g., openshift/oadp-must-gather). Uses MUST_GATHER_BRANCH (default: main).
ifeq ($(MUST_GATHER_REPO),)
	$(error MUST_GATHER_REPO is required (e.g., openshift/oadp-must-gather))
endif
	$(eval MUST_GATHER_TMP := $(shell mktemp -d))
	git clone --depth=1 --branch $(MUST_GATHER_BRANCH) https://github.com/$(MUST_GATHER_REPO).git $(MUST_GATHER_TMP)
	$(CONTAINER_TOOL) build --load -t $(MUST_GATHER_IMAGE) -f $(MUST_GATHER_TMP)/Dockerfile $(MUST_GATHER_TMP)
	$(CONTAINER_TOOL) push $(MUST_GATHER_IMAGE)
	rm -rf $(MUST_GATHER_TMP)
	@echo "Must-gather image built and pushed: $(MUST_GATHER_IMAGE)"

.PHONY: test-e2e
test-e2e: test-e2e-setup install-ginkgo $(if $(MUST_GATHER_REPO),build-must-gather) ## Run E2E tests against OADP operator installed in cluster. For more information, check docs/developer/testing/TESTING.md
	MUST_GATHER_IMAGE=$(MUST_GATHER_IMAGE) \
	ginkgo run -mod=mod $(GINKGO_FLAGS) $(GINKGO_ARGS) tests/e2e/ -- \
	-settings=$(SETTINGS_TMP)/oadpcreds \
	-provider=$(CLUSTER_TYPE) \
	-credentials=$(OADP_CRED_FILE) \
	-ci_cred_file=$(CI_CRED_FILE) \
	-velero_namespace=$(OADP_TEST_NAMESPACE) \
	-velero_instance_name=$(VELERO_INSTANCE_NAME) \
	-artifact_dir=$(ARTIFACT_DIR) \
	-kvm_emulation=$(KVM_EMULATION) \
	-hco_upstream=$(HCO_UPSTREAM) \
	-hco_community=$(HCO_COMMUNITY) \
	-hco_index_tag=$(HCO_INDEX_TAG) \
	-skipMustGather=$(SKIP_MUST_GATHER) \
	$(HCP_EXTERNAL_ARGS) \
	|| EXIT_CODE=$$?; \
	exit $${EXIT_CODE:-0}

.PHONY: test-e2e-cleanup
test-e2e-cleanup: login-required
	$(OC_CLI) delete volumesnapshotcontent --all
	$(OC_CLI) delete volumesnapshotclass oadp-example-snapclass --ignore-not-found=true
	$(OC_CLI) delete backup -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete backuprepository -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete downloadrequest -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete podvolumerestore -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete dataupload -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete datadownload -n $(OADP_TEST_NAMESPACE) --all
	$(OC_CLI) delete restore -n $(OADP_TEST_NAMESPACE) --all --wait=false
	for restore_name in $(shell $(OC_CLI) get restore -n $(OADP_TEST_NAMESPACE) -o name);do $(OC_CLI) patch "$$restore_name" -n $(OADP_TEST_NAMESPACE) -p '{"metadata":{"finalizers":null}}' --type=merge;done
	$(OC_CLI) delete ns mongo-persistent --ignore-not-found=true
	$(OC_CLI) delete ns mysql-persistent --ignore-not-found=true
	rm -rf $(SETTINGS_TMP)

.PHONY: update-non-admin-manifests
update-non-admin-manifests: NON_ADMIN_CONTROLLER_IMG?=quay.io/konveyor/oadp-non-admin:latest
update-non-admin-manifests: yq ## Update Non Admin Controller (NAC) manifests shipped with OADP, from NON_ADMIN_CONTROLLER_PATH
ifeq ($(NON_ADMIN_CONTROLLER_PATH),)
	$(error You must set NON_ADMIN_CONTROLLER_PATH to run this command)
endif
	@for file_name in $(shell ls $(NON_ADMIN_CONTROLLER_PATH)/config/crd/bases);do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/crd/bases/$$file_name $(shell pwd)/config/crd/bases/$$file_name && \
		grep -q "\- bases/$$file_name" $(shell pwd)/config/crd/kustomization.yaml || \
		$(SED) -i "s%resources:%resources:\n- bases/$$file_name%" $(shell pwd)/config/crd/kustomization.yaml;done
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_NON_ADMIN_CONTROLLER") |= .value="$(NON_ADMIN_CONTROLLER_IMG)"' config/manager/manager.yaml
	@mkdir -p $(shell pwd)/config/non-admin-controller_rbac
	@for file_name in $(shell grep -I '^\-' $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/$$file_name $(shell pwd)/config/non-admin-controller_rbac/$$file_name;done
	@cp $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/kustomization.yaml $(shell pwd)/config/non-admin-controller_rbac/kustomization.yaml
	@for file_name in $(shell grep -I '^\-' $(NON_ADMIN_CONTROLLER_PATH)/config/samples/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/samples/$$file_name $(shell pwd)/config/samples/$$file_name && \
		grep -q "\- $$file_name" $(shell pwd)/config/samples/kustomization.yaml || \
		$(SED) -i "s%resources:%resources:\n- $$file_name%" $(shell pwd)/config/samples/kustomization.yaml;done
	@make bundle

.PHONY: update-vmfr-manifests
update-vmfr-manifests: VMFR_CONTROLLER_IMG?=quay.io/konveyor/oadp-vm-file-restore:latest
update-vmfr-manifests: VMFR_ACCESS_IMG?=quay.io/konveyor/oadp-vmfr-access:latest
update-vmfr-manifests: VMFR_SSH_IMG?=quay.io/konveyor/oadp-vmfr-access-sshd:latest
update-vmfr-manifests: VMFR_BROWSER_IMG?=quay.io/konveyor/oadp-vmfr-access-filebrowser:latest
update-vmfr-manifests: yq ## Update VM File Restore (VMFR) manifests shipped with OADP, from VMFR_CONTROLLER_PATH
ifeq ($(VMFR_CONTROLLER_PATH),)
	$(error You must set VMFR_CONTROLLER_PATH to run this command)
endif
	@for file_name in $(shell ls $(VMFR_CONTROLLER_PATH)/config/crd/bases);do \
		cp $(VMFR_CONTROLLER_PATH)/config/crd/bases/$$file_name $(shell pwd)/config/crd/bases/$$file_name && \
		grep -q "\- bases/$$file_name" $(shell pwd)/config/crd/kustomization.yaml || \
		$(SED) -i "s%resources:%resources:\n- bases/$$file_name%" $(shell pwd)/config/crd/kustomization.yaml;done
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER") |= .value="$(VMFR_CONTROLLER_IMG)"' config/manager/manager.yaml
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_VM_FILE_RESTORE_ACCESS") |= .value="$(VMFR_ACCESS_IMG)"' config/manager/manager.yaml
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_VM_FILE_RESTORE_SSH") |= .value="$(VMFR_SSH_IMG)"' config/manager/manager.yaml
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_VM_FILE_RESTORE_BROWSER") |= .value="$(VMFR_BROWSER_IMG)"' config/manager/manager.yaml
	@mkdir -p $(shell pwd)/config/vm-file-restore-controller_rbac
	@for file_name in $(shell grep -I '^\-' $(VMFR_CONTROLLER_PATH)/config/rbac/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(VMFR_CONTROLLER_PATH)/config/rbac/$$file_name $(shell pwd)/config/vm-file-restore-controller_rbac/$$file_name;done
	@cp $(VMFR_CONTROLLER_PATH)/config/rbac/kustomization.yaml $(shell pwd)/config/vm-file-restore-controller_rbac/kustomization.yaml
	@$(SED) -i '1i namePrefix: oadp-vm-file-restore-' $(shell pwd)/config/vm-file-restore-controller_rbac/kustomization.yaml
	@for file_name in $(shell grep -I '^\-' $(VMFR_CONTROLLER_PATH)/config/samples/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(VMFR_CONTROLLER_PATH)/config/samples/$$file_name $(shell pwd)/config/samples/$$file_name && \
		grep -q "\- $$file_name" $(shell pwd)/config/samples/kustomization.yaml || \
		$(SED) -i "s%resources:%resources:\n- $$file_name%" $(shell pwd)/config/samples/kustomization.yaml;done
	@make bundle

.PHONY: update-kubevirt-datamover-manifests
update-kubevirt-datamover-manifests: KUBEVIRT_DATAMOVER_CONTROLLER_IMG?=quay.io/konveyor/kubevirt-datamover-controller:latest
update-kubevirt-datamover-manifests: yq ## Update KubeVirt Datamover Controller RBAC manifests shipped with OADP, from KUBEVIRT_DATAMOVER_PATH
ifeq ($(KUBEVIRT_DATAMOVER_PATH),)
	$(error You must set KUBEVIRT_DATAMOVER_PATH to run this command)
endif
	$(YQ) -i 'select(.kind == "Deployment")|= .spec.template.spec.containers[0].env |= .[] |= select(.name == "RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER") |= .value="$(KUBEVIRT_DATAMOVER_CONTROLLER_IMG)"' config/manager/manager.yaml
	@mkdir -p $(shell pwd)/config/kubevirt-datamover-controller_rbac
	@for file_name in $(shell grep -I '^\-' $(KUBEVIRT_DATAMOVER_PATH)/config/rbac/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(KUBEVIRT_DATAMOVER_PATH)/config/rbac/$$file_name $(shell pwd)/config/kubevirt-datamover-controller_rbac/$$file_name;done
	@cp $(KUBEVIRT_DATAMOVER_PATH)/config/rbac/kustomization.yaml $(shell pwd)/config/kubevirt-datamover-controller_rbac/kustomization.yaml
	@$(SED) -i '1i namePrefix: oadp-kubevirt-datamover-' $(shell pwd)/config/kubevirt-datamover-controller_rbac/kustomization.yaml
	@make bundle
