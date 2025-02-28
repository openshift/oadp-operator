# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
DEFAULT_VERSION := 99.0.0
VERSION ?= $(DEFAULT_VERSION)

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS = "stable"
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL = "stable"
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
BUNDLE_GEN_FLAGS ?= -q --extra-service-accounts "velero,non-admin-controller" --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.34.2

# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-operator:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29 # Kubernetes version from OpenShift 4.16.x https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt -mod=mod ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet -mod=mod ./...

# If test results in prow are different, it is because the environment used.
# You can simulate their env by running
# docker run --platform linux/amd64 -w $PWD -v $PWD:$PWD -it registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.14 sh -c "make test"
# where the image corresponds to the prow config for the test job, https://github.com/openshift/release/blob/master/ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master.yaml#L1-L5
# to login to registry cluster follow https://docs.ci.openshift.org/docs/how-tos/use-registries-in-build-farm/#how-do-i-log-in-to-pull-images-that-require-authentication
# If bin/ contains binaries of different arch, you may remove them so the container can install their arch.
.PHONY: test
test: vet envtest ## Run unit tests; run Go linters checks; check if api and bundle folders are up to date; and check if go dependencies are valid
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -mod=mod $(shell go list -mod=mod ./... | grep -v /tests/e2e) -coverprofile cover.out
	@make lint
	@make api-isupdated
	@make bundle-isupdated
	@make check-go-dependencies

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.55.2
.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))

.PHONY: lint
lint: golangci-lint ## Run Go linters checks against all project's Go files.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Fix Go linters issues.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
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
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v5.2.1
CONTROLLER_TOOLS_VERSION ?= v0.14.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6)

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifneq ($(shell $(OPERATOR_SDK) version | cut -d'"' -f2),$(OPERATOR_SDK_VERSION))
	set -e; \
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK);
endif

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
	sed -e 's/    createdAt: .*/$(shell grep -I '^    createdAt: ' bundle/manifests/oadp-operator.clusterserviceversion.yaml)/' bundle/manifests/oadp-operator.clusterserviceversion.yaml > bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp
	mv bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp bundle/manifests/oadp-operator.clusterserviceversion.yaml

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build --load -f bundle.Dockerfile -t $(BUNDLE_IMG) . $(DOCKER_BUILD_ARGS)

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM ?= $(LOCALBIN)/opm
OPM_VERSION ?= v1.23.0
opm: ## Download opm locally if necessary.
ifneq ($(shell $(OPM) version | cut -d'"' -f2),$(OPM_VERSION))
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM)
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

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
check-go-dependencies:
	@cp -r ./ $(TEMP) && cd $(TEMP) && go mod tidy && cd - && diff -ruN ./ $(TEMP)/ && echo "go dependencies checked" || (echo "go dependencies are out of date, run 'go mod tidy' to update" && exit 1)
	@chmod -R 777 $(TEMP) && rm -rf $(TEMP)
	go mod verify

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
    endif
endif
submit-coverage:
	curl -Os https://uploader.codecov.io/latest/$(OS_String)/codecov
	chmod +x codecov
	./codecov -C $(shell git rev-parse HEAD) -r openshift/oadp-operator --nonZero
	rm -f codecov

# go-install-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install -mod=mod $(2) ;\
rm -rf $$TMP_DIR ;\
}
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

.PHONY: deploy-olm
deploy-olm: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h # Set target specific variable
deploy-olm: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h # Set target specific variable
deploy-olm: DEPLOY_TMP:=$(shell mktemp -d)/ # Set target specific variable
deploy-olm: undeploy-olm ## Build current branch operator image, bundle image, push and install via OLM. For more information, check docs/developer/install_from_source.md
	@echo "DEPLOY_TMP: $(DEPLOY_TMP)"
	# build and push operator and bundle image
	# use $(OPERATOR_SDK) to install bundle to authenticated cluster
	cp -r . $(DEPLOY_TMP) && cd $(DEPLOY_TMP) && \
	IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle bundle-build bundle-push; \
	chmod -R 777 $(DEPLOY_TMP) && rm -rf $(DEPLOY_TMP)
	$(OPERATOR_SDK) run bundle --security-context-config restricted $(THIS_BUNDLE_IMAGE) --namespace $(OADP_TEST_NAMESPACE)

.PHONY: undeploy-olm
undeploy-olm: login-required operator-sdk ## Uninstall current branch operator via OLM
	$(OC_CLI) whoami # Check if logged in
	$(OC_CLI) create namespace $(OADP_TEST_NAMESPACE) || true
	$(OPERATOR_SDK) cleanup oadp-operator --namespace $(OADP_TEST_NAMESPACE)

# A valid Git branch from https://github.com/openshift/oadp-operator
PREVIOUS_CHANNEL ?= oadp-1.4
# Go version in go.mod in that branch
PREVIOUS_CHANNEL_GO_VERSION ?= 1.22

.PHONY: catalog-test-upgrade
catalog-test-upgrade: PREVIOUS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-previous-$(GIT_REV):1h
catalog-test-upgrade: PREVIOUS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-previous-bundle-$(GIT_REV):1h
catalog-test-upgrade: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h
catalog-test-upgrade: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h
catalog-test-upgrade: CATALOG_IMAGE?=ttl.sh/oadp-operator-catalog-$(GIT_REV):1h
catalog-test-upgrade: opm login-required ## Prepare a catalog image with two channels: PREVIOUS_CHANNEL and from current branch. For more information, check docs/developer/testing/test_oadp_version_upgrade.md
	mkdir test-upgrade && rsync -a --exclude=test-upgrade ./ test-upgrade/current
	git clone --depth=1 git@github.com:openshift/oadp-operator.git -b $(PREVIOUS_CHANNEL) test-upgrade/$(PREVIOUS_CHANNEL)
	cd test-upgrade/$(PREVIOUS_CHANNEL) && \
		echo -e "FROM golang:$(PREVIOUS_CHANNEL_GO_VERSION)\nRUN useradd --create-home dev\nUSER dev\nWORKDIR /home/dev/$(PREVIOUS_CHANNEL)" | $(CONTAINER_TOOL) image build --tag catalog-test-upgrade - && \
		$(CONTAINER_TOOL) container run -u $(shell id -u):$(shell id -g) -v $(shell pwd)/test-upgrade/$(PREVIOUS_CHANNEL):/home/dev/$(PREVIOUS_CHANNEL) --rm catalog-test-upgrade make bundle IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) && \
		sed -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	cd test-upgrade/current && IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) make bundle && \
		sed -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --bundles $(PREVIOUS_BUNDLE_IMAGE),$(THIS_BUNDLE_IMAGE) --tag $(CATALOG_IMAGE)
	$(CONTAINER_TOOL) push $(CATALOG_IMAGE)
	echo -e "apiVersion: operators.coreos.com/v1alpha1\nkind: CatalogSource\nmetadata:\n  name: oadp-operator-catalog-test-upgrade\n  namespace: openshift-marketplace\nspec:\n  sourceType: grpc\n  image: $(CATALOG_IMAGE)" | $(OC_CLI) create -f -
	chmod -R 777 test-upgrade && rm -rf test-upgrade && $(CONTAINER_TOOL) image rm catalog-test-upgrade

.PHONY: install-ginkgo
install-ginkgo: ## Make sure ginkgo is in $GOPATH/bin
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

OPENSHIFT_CI ?= true
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
	VSL_REGION="$(VSL_REGION)" \
	BSL_REGION="$(BSL_REGION)" \
	BSL_AWS_PROFILE="$(BSL_AWS_PROFILE)" \
	/bin/bash "tests/e2e/scripts/$(CLUSTER_TYPE)_settings.sh"

VELERO_INSTANCE_NAME ?= velero-test
ARTIFACT_DIR ?= /tmp
HCO_UPSTREAM ?= false
TEST_VIRT ?= false
TEST_UPGRADE ?= false
TEST_FILTER = (($(shell echo '! aws && ! gcp && ! azure && ! ibmcloud' | \
sed -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")) || $(CLUSTER_TYPE))
#TEST_FILTER := $(shell echo '! aws && ! gcp && ! azure' | sed -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")
ifeq ($(TEST_VIRT),true)
	TEST_FILTER += && (virt)
else
	TEST_FILTER += && (! virt)
endif
ifeq ($(TEST_UPGRADE),true)
	TEST_FILTER += && (upgrade)
else
	TEST_FILTER += && (! upgrade)
endif

.PHONY: test-e2e
test-e2e: test-e2e-setup install-ginkgo ## Run E2E tests against OADP operator installed in cluster. For more information, check docs/developer/testing/TESTING.md
	ginkgo run -mod=mod tests/e2e/ -- \
	-settings=$(SETTINGS_TMP)/oadpcreds \
	-provider=$(CLUSTER_TYPE) \
	-credentials=$(OADP_CRED_FILE) \
	-ci_cred_file=$(CI_CRED_FILE) \
	-velero_namespace=$(OADP_TEST_NAMESPACE) \
	-velero_instance_name=$(VELERO_INSTANCE_NAME) \
	-artifact_dir=$(ARTIFACT_DIR) \
	-oc_cli=$(OC_CLI) \
	-kvm_emulation=$(KVM_EMULATION) \
	-hco_upstream=$(HCO_UPSTREAM) \
	--ginkgo.vv \
	--ginkgo.no-color=$(OPENSHIFT_CI) \
	--ginkgo.label-filter="$(TEST_FILTER)" \
	--ginkgo.junit-report="junit_report.xml" \
	--ginkgo.timeout=2h \
	$(GINKGO_ARGS)

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
	rm -rf $(SETTINGS_TMP)

.PHONY: update-non-admin-manifests
update-non-admin-manifests: NON_ADMIN_CONTROLLER_IMG?=quay.io/konveyor/oadp-non-admin:latest
update-non-admin-manifests: ## Update Non Admin Controller (NAC) manifests shipped with OADP, from NON_ADMIN_CONTROLLER_PATH
ifeq ($(NON_ADMIN_CONTROLLER_PATH),)
	$(error You must set NON_ADMIN_CONTROLLER_PATH to run this command)
endif
	@for file_name in $(shell ls $(NON_ADMIN_CONTROLLER_PATH)/config/crd/bases);do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/crd/bases/$$file_name $(shell pwd)/config/crd/bases/$$file_name && \
		grep -q "\- bases/$$file_name" $(shell pwd)/config/crd/kustomization.yaml || \
		sed -i "s%resources:%resources:\n- bases/$$file_name%" $(shell pwd)/config/crd/kustomization.yaml;done
	@sed -i "$(shell grep -Inr 'RELATED_IMAGE_NON_ADMIN_CONTROLLER' $(shell pwd)/config/manager/manager.yaml | awk -F':' '{print $$1+1}')s%.*%            value: $(NON_ADMIN_CONTROLLER_IMG)%" $(shell pwd)/config/manager/manager.yaml
	@mkdir -p $(shell pwd)/config/non-admin-controller_rbac
	@for file_name in $(shell grep -I '^\-' $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/$$file_name $(shell pwd)/config/non-admin-controller_rbac/$$file_name;done
	@cp $(NON_ADMIN_CONTROLLER_PATH)/config/rbac/kustomization.yaml $(shell pwd)/config/non-admin-controller_rbac/kustomization.yaml
	@for file_name in $(shell grep -I '^\-' $(NON_ADMIN_CONTROLLER_PATH)/config/samples/kustomization.yaml | awk -F'- ' '{print $$2}');do \
		cp $(NON_ADMIN_CONTROLLER_PATH)/config/samples/$$file_name $(shell pwd)/config/samples/$$file_name && \
		grep -q "\- $$file_name" $(shell pwd)/config/samples/kustomization.yaml || \
		sed -i "s%resources:%resources:\n- $$file_name%" $(shell pwd)/config/samples/kustomization.yaml;done
	@make bundle
