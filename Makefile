OADP_TEST_NAMESPACE ?= openshift-adp

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

# Misc
OPENSHIFT_CI ?= true
VELERO_INSTANCE_NAME ?= velero-test
ARTIFACT_DIR ?= /tmp
OC_CLI = $(shell which oc)
TEST_VIRT ?= false
TEST_HCP ?= false
TEST_UPGRADE ?= false

# TOOL VERSIONS
# All version-related variables are defined here for easy maintenance
DEFAULT_VERSION := 1.4.9
VERSION ?= $(DEFAULT_VERSION) # the version of the operator
OPERATOR_SDK_VERSION ?= v1.34.2
ENVTEST_K8S_VERSION = 1.29 # Kubernetes version from OpenShift 4.16.x
GOLANGCI_LINT_VERSION ?= v1.54.2
KUSTOMIZE_VERSION ?= v4.5.5
CONTROLLER_TOOLS_VERSION ?= v0.16.5
OPM_VERSION ?= v1.15.1
YQ_VERSION ?= v4.28.1
BRANCH_VERSION = oadp-1.4
PREVIOUS_CHANNEL ?= oadp-1.3
PREVIOUS_CHANNEL_GO_VERSION ?= 1.21
# Extract the toolchain directive from go.mod
GO_TOOLCHAIN_VERSION := $(shell grep -E "^toolchain" go.mod | awk '{print $$2}')

# Set GOPROXY to use the proxy.golang.org mirror to avoid rate limiting issues
export GOPROXY ?= https://proxy.golang.org,direct
# Set GOSUMDB to use the sum.golang.org checksum database to avoid checksum mismatch issues
export GOSUMDB ?= sum.golang.org


# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS = "stable-1.4"
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL = "stable-1.4"
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

# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-operator:oadp-1.4

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Tool Definitions
GOLANGCI_LINT = $(LOCALBIN)/$(BRANCH_VERSION)/golangci-lint
KUSTOMIZE ?= $(LOCALBIN)/$(BRANCH_VERSION)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/$(BRANCH_VERSION)/controller-gen
OPERATOR_SDK ?= $(LOCALBIN)/$(BRANCH_VERSION)/operator-sdk
OPM ?= $(LOCALBIN)/$(BRANCH_VERSION)/opm
YQ = $(LOCALBIN)/$(BRANCH_VERSION)/yq

ifdef CLI_DIR
	OC_CLI = ${CLI_DIR}/oc
endif
# makes CLUSTER_TYPE quieter when unauthenticated
CLUSTER_TYPE_SHELL := $(shell $(OC_CLI) get infrastructures cluster -o jsonpath='{.status.platform}' 2> /dev/null | tr A-Z a-z)
CLUSTER_TYPE ?= $(CLUSTER_TYPE_SHELL)
CLUSTER_OS = $(shell $(OC_CLI) get node -o jsonpath='{.items[0].status.nodeInfo.operatingSystem}' 2> /dev/null)
CLUSTER_ARCH = $(shell $(OC_CLI) get node -o jsonpath='{.items[0].status.nodeInfo.architecture}' 2> /dev/null)

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

# Kubernetes version from OpenShift 4.16.x https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
DEFAULT_VERSION := 1.4.9
VERSION ?= $(DEFAULT_VERSION)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# openshift.io/oadp-operator-bundle:$VERSION and openshift.io/oadp-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= openshift.io/oadp-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-operator:oadp-1.4
CRD_OPTIONS ?= "crd"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

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

.PHONY: check-go
check-go: ## Check if go binary is available in PATH
	@if ! command -v go >/dev/null 2>&1; then \
		printf "\033[31m✗ Error: 'go' binary not found in PATH\033[0m\n"; \
		printf "Please install Go from https://golang.org/dl/ and ensure it's in your PATH\n"; \
		exit 1; \
	fi
	@printf "\033[32m✓ Go binary found in PATH\033[0m\n"

.PHONY: versions
versions: check-go ## Display all variables containing 'version' in their name.
	@printf "\033[36m%-30s\033[0m %s\n" "GO_VERSION" "$$(go version | awk '{print $$3}')"
	@printf "\n\033[1m%-30s %-20s %s\033[0m\n" "Tool and Project Versions:" "Value" "Used by Targets"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "DEFAULT_VERSION" "$(DEFAULT_VERSION)" "bundle-isupdated"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "VERSION" "$(VERSION)" "bundle, catalog-build, deploy-olm, undeploy-olm"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "OPERATOR_SDK_VERSION" "$(OPERATOR_SDK_VERSION)" "operator-sdk"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "ENVTEST_K8S_VERSION" "$(ENVTEST_K8S_VERSION)" "test"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "GOLANGCI_LINT_VERSION" "$(GOLANGCI_LINT_VERSION)" "golangci-lint"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "KUSTOMIZE_VERSION" "$(KUSTOMIZE_VERSION)" "kustomize"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "CONTROLLER_TOOLS_VERSION" "$(CONTROLLER_TOOLS_VERSION)" "controller-gen"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "OPM_VERSION" "$(OPM_VERSION)" "opm"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "YQ_VERSION" "$(YQ_VERSION)" "yq"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "PREVIOUS_CHANNEL" "$(PREVIOUS_CHANNEL)" "catalog-test-upgrade"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "PREVIOUS_CHANNEL_GO_VERSION" "$(PREVIOUS_CHANNEL_GO_VERSION)" "catalog-test-upgrade"
	@printf "\033[36m%-30s\033[0m %-20s %s\n" "GO_TOOLCHAIN_VERSION" "$(GO_TOOLCHAIN_VERSION)" "(informational only)"
	$(call CHECK_TOOL_VERSION,Operator-SDK,$(OPERATOR_SDK),$(OPERATOR_SDK) version 2>/dev/null | grep 'operator-sdk version' | cut -d'"' -f2,$(OPERATOR_SDK_VERSION),operator-sdk,OPERATOR_SDK_LOCAL)
	$(call CHECK_TOOL_VERSION,Controller-Gen,$(CONTROLLER_GEN),$(CONTROLLER_GEN) --version 2>/dev/null | grep 'Version:' | cut -d' ' -f2,$(CONTROLLER_TOOLS_VERSION),controller-gen,CONTROLLER_GEN_LOCAL)
	$(call CHECK_TOOL_VERSION,OPM,$(OPM),$(OPM) version 2>/dev/null | cut -d'"' -f2,$(OPM_VERSION),opm,OPM_LOCAL)
	$(call CHECK_TOOL_VERSION,Kustomize,$(KUSTOMIZE),$(KUSTOMIZE) version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown",$(KUSTOMIZE_VERSION),kustomize,KUSTOMIZE_LOCAL,elif [ "$$INSTALLED_VERSION" = "unknown" ]; then printf "\033[36m%-30s\033[0m %-20s %s\n" "KUSTOMIZE_LOCAL" "$$INSTALLED_VERSION" "ⓘ kustomize v4 build (expected $(KUSTOMIZE_VERSION))";)
	$(call CHECK_TOOL_VERSION,YQ,$(YQ),$(YQ) --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1,$(YQ_VERSION:v%=%),yq,YQ_LOCAL)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	# Commenting out default which overwrites scoped config/rbac/role.yaml
	# GOFLAGS="-mod=mod" $(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	# run make nullables to generate nullable fields after all manifest changesin dependent targets.
	# It's not included here because `test` and `bundle` target have different yaml styes.
	# To keep dpa CRD the same, nullables have been added to test and bundle target separately.

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: check-go ## Run go fmt against code.
	go fmt -mod=mod ./...

vet: check-go ## Run go vet against code.
	go vet -mod=mod ./...

ENVTEST := $(shell pwd)/bin/setup-envtest
ENVTESTPATH = $(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)
ifeq ($(shell $(ENVTEST) list | grep $(ENVTEST_K8S_VERSION)),)
	ENVTESTPATH = $(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)
endif
$(ENVTEST): ## Download envtest-setup locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6)

.PHONY: envtest
envtest: $(ENVTEST)

# If test results in prow are different, it is because the environment used.
# You can simulate their env by running
# docker run --platform linux/amd64 -w $PWD -v $PWD:$PWD -it registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.14 sh -c "make test"
# where the image corresponds to the prow config for the test job, https://github.com/openshift/release/blob/master/ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-master.yaml#L1-L5
# to login to registry cluster follow https://docs.ci.openshift.org/docs/how-tos/use-registries-in-build-farm/#how-do-i-log-in-to-pull-images-that-require-authentication
# If bin/ contains binaries of different arch, you may remove them so the container can install their arch.
.PHONY: test
test: check-go vet envtest ## Run Go linter and unit tests and check Go code format and if api and bundle folders are up to date.
	KUBEBUILDER_ASSETS="$(ENVTESTPATH)" go test -mod=mod $(shell go list -mod=mod ./... | grep -v /tests/e2e) -coverprofile cover.out
	@make fmt-isupdated
	@make api-isupdated
	@make bundle-isupdated

.PHONY: fmt-isupdated
fmt-isupdated: TEMP:= $(shell mktemp -d)
fmt-isupdated:
	@cp -r ./ $(TEMP) && cd $(TEMP) && make fmt && cd - && diff -ruN . $(TEMP)/ && echo "Go code is formatted" || (echo "Go code is not formatted, run 'make fmt' to format" && exit 1)
	@chmod -R 777 $(TEMP) && rm -rf $(TEMP)

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

##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

# If using podman machine, and host platform is not linux/amd64 run
# - podman machine ssh sudo rpm-ostree install qemu-user-static && sudo systemctl reboot
# from: https://github.com/containers/podman/issues/12144#issuecomment-955760527
# related enhancements that may remove the need to manually install qemu-user-static https://bugzilla.redhat.com/show_bug.cgi?id=2061584
DOCKER_BUILD_ARGS ?= --platform=linux/amd64
ifneq ($(CLUSTER_TYPE),)
	DOCKER_BUILD_ARGS = --platform=$(CLUSTER_OS)/$(CLUSTER_ARCH)
endif
docker-build: ## Build docker image with the manager.
	docker build -t $(IMG) . $(DOCKER_BUILD_ARGS)

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

VELERO_ROLE_TMP?=/tmp/oadp-make-deploy
velero-role-tmp: kustomize
	mkdir -p $(VELERO_ROLE_TMP)
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-service_account.yaml > $(VELERO_ROLE_TMP)/velero-service_account.yaml
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-role.yaml > $(VELERO_ROLE_TMP)/velero-role.yaml
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-role_binding.yaml > $(VELERO_ROLE_TMP)/velero-role_binding.yaml
velero-role-tmp-cleanup:
	rm -rf $(VELERO_ROLE_TMP)
apply-velerosa-role: velero-role-tmp
	kubectl apply -f $(VELERO_ROLE_TMP)/velero-service_account.yaml
	kubectl apply -f $(VELERO_ROLE_TMP)/velero-role.yaml
	kubectl apply -f $(VELERO_ROLE_TMP)/velero-role_binding.yaml
	VELERO_ROLE_TMP=$(VELERO_ROLE_TMP) make velero-role-tmp-cleanup
unapply-velerosa-role: velero-role-tmp
	kubectl delete -f $(VELERO_ROLE_TMP)/velero-service_account.yaml
	kubectl delete -f $(VELERO_ROLE_TMP)/velero-role.yaml
	kubectl delete -f $(VELERO_ROLE_TMP)/velero-role_binding.yaml
	VELERO_ROLE_TMP=$(VELERO_ROLE_TMP) make velero-role-tmp-cleanup

build-deploy: THIS_IMAGE=ttl.sh/oadp-operator-$(shell git rev-parse --short HEAD):1h # Set target specific variable
build-deploy: ## Build current branch image and deploy controller to the k8s cluster specified in ~/.kube/config.
	IMG=$(THIS_IMAGE) make docker-build docker-push deploy

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5)



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
	./codecov > tmp.results 2> tmp.err
	cat tmp.results || echo "tmp.results not found"
	cat tmp.err || echo "tmp.err not found"
	@echo $$(cat tmp.results | grep 'resultURL' -c)
	@echo $$(cat tmp.err | grep 'please specify sha and slug manually' -c)
	if [ $$(cat tmp.err | grep 'please specify sha and slug manually' -c) == 1 ]; then \
		echo "specifying sha and slug manually" && ./codecov -C $(shell git rev-parse HEAD) -r openshift/oadp-operator; \
	fi
	rm -f codecov tmp.*

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





.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	GOFLAGS="-mod=mod" $(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && GOFLAGS="-mod=mod" $(KUSTOMIZE) edit set image controller=$(IMG)
	GOFLAGS="-mod=mod" $(KUSTOMIZE) build config/manifests | GOFLAGS="-mod=mod" $(OPERATOR_SDK) generate bundle -q --extra-service-accounts "velero" --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	@make nullables
	# Copy updated bundle.Dockerfile to CI's Dockerfile.bundle
	# TODO: update CI to use generated one
	cp bundle.Dockerfile build/Dockerfile.bundle
	GOFLAGS="-mod=mod" $(OPERATOR_SDK) bundle validate ./bundle
	sed -e 's/    createdAt: .*/$(shell grep -I '^    createdAt: ' bundle/manifests/oadp-operator.clusterserviceversion.yaml)/' bundle/manifests/oadp-operator.clusterserviceversion.yaml > bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp
	mv bundle/manifests/oadp-operator.clusterserviceversion.yaml.tmp bundle/manifests/oadp-operator.clusterserviceversion.yaml

.PHONY: nullables
nullables:
	@make nullable-crds-bundle nullable-crds-config # patch nullables in CRDs

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

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) . $(DOCKER_BUILD_ARGS)

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

GIT_REV:=$(shell git rev-parse --short HEAD)
.PHONY: deploy-olm
deploy-olm: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h # Set target specific variable
deploy-olm: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h # Set target specific variable
deploy-olm: DEPLOY_TMP:=$(shell mktemp -d)/ # Set target specific variable
deploy-olm: operator-sdk undeploy-olm ## Build current branch operator image, bundle image, push and install via OLM
	@echo "DEPLOY_TMP: $(DEPLOY_TMP)"
	# build and push operator and bundle image
	# use $(OPERATOR_SDK) to install bundle to authenticated cluster
	cp -r . $(DEPLOY_TMP) && cd $(DEPLOY_TMP) && \
	IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle bundle-build bundle-push; \
	chmod -R 777 ${DEPLOY_TMP} && rm -rf $(DEPLOY_TMP)
	$(OPERATOR_SDK) run bundle --security-context-config restricted $(THIS_BUNDLE_IMAGE) --namespace $(OADP_TEST_NAMESPACE)

.PHONY: undeploy-olm
undeploy-olm: login-required ## Uninstall current branch operator via OLM
	$(OC_CLI) whoami # Check if logged in
	$(OC_CLI) create namespace $(OADP_TEST_NAMESPACE) || true
	$(OPERATOR_SDK) cleanup oadp-operator --namespace $(OADP_TEST_NAMESPACE)

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
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# For testing oeprator upgrade
# opm upgrade
catalog-build-replaces: opm ## Build a catalog image using replace mode
	$(OPM) index add --container-tool docker --mode replaces --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

# A valid Git branch from https://github.com/openshift/oadp-operator
PREVIOUS_CHANNEL ?= oadp-1.3
# Go version in go.mod in that branch
PREVIOUS_CHANNEL_GO_VERSION ?= 1.20

.PHONY: catalog-test-upgrade
catalog-test-upgrade: PREVIOUS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-previous-$(GIT_REV):1h
catalog-test-upgrade: PREVIOUS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-previous-bundle-$(GIT_REV):1h
catalog-test-upgrade: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h
catalog-test-upgrade: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h
catalog-test-upgrade: CATALOG_IMAGE?=ttl.sh/oadp-operator-catalog-$(GIT_REV):1h
catalog-test-upgrade: opm login-required ## Prepare a catalog image with two channels: PREVIOUS_CHANNEL and from current branch
	mkdir test-upgrade && rsync -a --exclude=test-upgrade ./ test-upgrade/current
	git clone --depth=1 git@github.com:openshift/oadp-operator.git -b $(PREVIOUS_CHANNEL) test-upgrade/$(PREVIOUS_CHANNEL)
	cd test-upgrade/$(PREVIOUS_CHANNEL) && \
		echo -e "FROM golang:$(PREVIOUS_CHANNEL_GO_VERSION)\nRUN useradd --create-home dev\nUSER dev\nWORKDIR /home/dev/$(PREVIOUS_CHANNEL)" | docker image build --tag catalog-test-upgrade - && \
		docker container run -u $(shell id -u):$(shell id -g) -v $(shell pwd)/test-upgrade/$(PREVIOUS_CHANNEL):/home/dev/$(PREVIOUS_CHANNEL) --rm catalog-test-upgrade make bundle IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) && \
		sed -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(PREVIOUS_OPERATOR_IMAGE) BUNDLE_IMG=$(PREVIOUS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	cd test-upgrade/current && IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) make bundle && \
		sed -i '/replaces:/d' ./bundle/manifests/oadp-operator.clusterserviceversion.yaml && \
		IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle-build bundle-push && cd -
	$(OPM) index add --container-tool docker --bundles $(PREVIOUS_BUNDLE_IMAGE),$(THIS_BUNDLE_IMAGE) --tag $(CATALOG_IMAGE)
	docker push $(CATALOG_IMAGE)
	echo -e "apiVersion: operators.coreos.com/v1alpha1\nkind: CatalogSource\nmetadata:\n  name: oadp-operator-catalog-test-upgrade\n  namespace: openshift-marketplace\nspec:\n  sourceType: grpc\n  image: $(CATALOG_IMAGE)" | $(OC_CLI) create -f -
	chmod -R 777 test-upgrade && rm -rf test-upgrade && docker image rm catalog-test-upgrade

.PHONY: login-required
login-required:
ifeq ($(CLUSTER_TYPE),)
	$(error You must be logged in to a cluster to run this command)
else
	$(info $$CLUSTER_TYPE is [${CLUSTER_TYPE}])
endif

.PHONY: install-ginkgo
install-ginkgo: check-go ## Make sure ginkgo is in $GOPATH/bin
	go install -v -mod=mod github.com/onsi/ginkgo/v2/ginkgo

OADP_BUCKET ?= $(shell cat $(OADP_BUCKET_FILE))
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
ifeq ($(TEST_HCP),true)
	TEST_FILTER += && (hcp)
else
	TEST_FILTER += && (! hcp)
endif
SETTINGS_TMP=/tmp/test-settings

.PHONY: test-e2e-setup
test-e2e-setup: login-required build-must-gather
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

.PHONY: test-e2e
test-e2e: test-e2e-setup install-ginkgo
	ginkgo run -mod=mod tests/e2e/ -- \
	-settings=$(SETTINGS_TMP)/oadpcreds \
	-provider=$(CLUSTER_TYPE) \
	-credentials=$(OADP_CRED_FILE) \
	-ci_cred_file=$(CI_CRED_FILE) \
	-velero_namespace=$(OADP_TEST_NAMESPACE) \
	-velero_instance_name=$(VELERO_INSTANCE_NAME) \
	-artifact_dir=$(ARTIFACT_DIR) \
	--ginkgo.vv \
	--ginkgo.no-color=$(OPENSHIFT_CI) \
	--ginkgo.label-filter="$(TEST_FILTER)" \
	--ginkgo.timeout=2h

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

# go-install-tool-branch will 'go install' any package $2 and install it to branch-specific directory $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool-branch
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2) to branch directory" ;\
GOBIN=$(dir $(1)) go install -mod=mod $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool-branch,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))
	@if [ -L "$(LOCALBIN)/golangci-lint" ]; then \
		unlink "$(LOCALBIN)/golangci-lint"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/golangci-lint" "$(LOCALBIN)/golangci-lint"

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool-branch,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION))
	@if [ -L "$(LOCALBIN)/kustomize" ]; then \
		unlink "$(LOCALBIN)/kustomize"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/kustomize" "$(LOCALBIN)/kustomize"

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool-branch,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION))
	@if [ -L "$(LOCALBIN)/controller-gen" ]; then \
		unlink "$(LOCALBIN)/controller-gen"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/controller-gen" "$(LOCALBIN)/controller-gen"

.PHONY: operator-sdk
operator-sdk: $(OPERATOR_SDK) ## Download operator-sdk locally if necessary.
$(OPERATOR_SDK): $(LOCALBIN)
	@if [ -f "$(OPERATOR_SDK)" ] && $(OPERATOR_SDK) version | grep -q $(OPERATOR_SDK_VERSION); then \
		echo "operator-sdk $(OPERATOR_SDK_VERSION) is already installed"; \
	else \
		set -e ;\
		mkdir -p $(dir $(OPERATOR_SDK)) ;\
		OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
		curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
		chmod +x $(OPERATOR_SDK) ;\
	fi
	@if [ -L "$(LOCALBIN)/operator-sdk" ]; then \
		unlink "$(LOCALBIN)/operator-sdk"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/operator-sdk" "$(LOCALBIN)/operator-sdk"

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM): $(LOCALBIN)
	@if [ -f "$(OPM)" ] && $(OPM) version | grep -q $(OPM_VERSION); then \
		echo "opm $(OPM_VERSION) is already installed"; \
	else \
		set -e ;\
		mkdir -p $(dir $(OPM)) ;\
		OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
		curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
		chmod +x $(OPM) ;\
	fi
	@if [ -L "$(LOCALBIN)/opm" ]; then \
		unlink "$(LOCALBIN)/opm"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/opm" "$(LOCALBIN)/opm"

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	$(call go-install-tool-branch,$(YQ),github.com/mikefarah/yq/v4@$(YQ_VERSION))
	@if [ -L "$(LOCALBIN)/yq" ]; then \
		unlink "$(LOCALBIN)/yq"; \
	fi
	@ln -sf "$(LOCALBIN)/$(BRANCH_VERSION)/yq" "$(LOCALBIN)/yq"

.PHONY: build-must-gather
build-must-gather: check-go ## Build OADP Must-gather binary must-gather/oadp-must-gather
	cd must-gather && go build -mod=mod -a -o oadp-must-gather cmd/main.go
