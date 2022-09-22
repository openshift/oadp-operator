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

# vsl secret
CREDS_SECRET_REF ?= cloud-credentials
# bucket file
OADP_BUCKET_FILE ?= ${OADP_CRED_DIR}/new-velero-bucket-name
# azure cluster resource file - only in CI
AZURE_RESOURCE_FILE ?= /var/run/secrets/ci.openshift.io/multi-stage/metadata.json
AZURE_CI_JSON_CRED_FILE ?= ${CLUSTER_PROFILE_DIR}/osServicePrincipal.json
AZURE_OADP_JSON_CRED_FILE ?= ${OADP_CRED_DIR}/azure-credentials

# Misc
OPENSHIFT_CI ?= true
VELERO_INSTANCE_NAME ?= velero-test
E2E_TIMEOUT_MULTIPLIER ?= 1
ARTIFACT_DIR ?= /tmp
OC_CLI = $(shell which oc)

ifdef CLI_DIR
	OC_CLI = ${CLI_DIR}/oc
endif

CLUSTER_TYPE ?= $(shell $(OC_CLI) get infrastructures cluster -o jsonpath='{.status.platform}' | awk '{print tolower($0)}')

ifeq ($(CLUSTER_TYPE), gcp)
	CI_CRED_FILE = ${CLUSTER_PROFILE_DIR}/gce.json
	OADP_CRED_FILE = ${OADP_CRED_DIR}/gcp-credentials
	CREDS_SECRET_REF = cloud-credentials-gcp
	OADP_BUCKET_FILE = ${OADP_CRED_DIR}/gcp-velero-bucket-name
endif

ifeq ($(CLUSTER_TYPE), azure4)
	CLUSTER_TYPE = azure
endif

ifeq ($(CLUSTER_TYPE), azure)
	CI_CRED_FILE = /tmp/ci-azure-credentials
	OADP_CRED_FILE = /tmp/oadp-azure-credentials
	CREDS_SECRET_REF = cloud-credentials-azure
	OADP_BUCKET_FILE = ${OADP_CRED_DIR}/azure-velero-bucket-name
endif

VELERO_PLUGIN ?= ${CLUSTER_TYPE}

ifeq ($(CLUSTER_TYPE), ibmcloud)
	VELERO_PLUGIN ?= aws
endif

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

.PHONY:ginkgo
ginkgo: # Make sure ginkgo is in $GOPATH/bin
	go get -d github.com/onsi/ginkgo/ginkgo
	go get -d github.com/onsi/ginkgo/v2/ginkgo
	go get -d github.com/onsi/gomega/...

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 1.1.0

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS = "stable,stable-1.1"
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL = "stable-1.1"
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

# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-operator:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

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

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	# Commenting out default which overwrites scoped config/rbac/role.yaml
	# GOFLAGS="-mod=mod" $(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	GOFLAGS="-mod=mod" $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt -mod=mod ./...

vet: ## Run go vet against code.
	go vet -mod=mod ./...

test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(ENVTESTPATH)" go test -mod=mod ./controllers/... ./pkg/... -coverprofile cover.out


ENVTEST = $(shell pwd)/bin/setup-envtest
ENVTESTPATH = $(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)
# if there is no native arch available, attempt to use amd64
ifeq ($(shell $(ENVTEST) list),)
	ENVTESTPATH = $(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)
endif
ci-test: ## This assumes "manifests generate fmt vet envtest" ran.
	KUBEBUILDER_ASSETS="$(ENVTESTPATH)" go test -mod=mod ./controllers/... -coverprofile cover.out


##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

# Development clusters require linux/amd64 OCI image
# Set platform to linux/amd64 regardless of host platform.
# If using podman machine, and host platform is not linux/amd64 run
# - podman machine ssh sudo rpm-ostree install qemu-user-static && sudo systemctl reboot
# from: https://github.com/containers/podman/issues/12144#issuecomment-955760527
# related enhancements that may remove the need to manually install qemu-user-static https://bugzilla.redhat.com/show_bug.cgi?id=2061584
DOCKER_BUILD_ARGS ?= --platform=linux/amd64
docker-build: test ## Build docker image with the manager.
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

# Deprecated in favor of `deploy-olm`
# deploy: manifests velero-role-tmp  ## Deploy controller to the K8s cluster specified in ~/.kube/config.
# 	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
# 	$(KUSTOMIZE) build config/default | kubectl apply -f -
# 	VELERO_ROLE_TMP=$(VELERO_ROLE_TMP) make apply-velerosa-role

# undeploy: velero-role-tmp ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
# 	VELERO_ROLE_TMP=$(VELERO_ROLE_TMP) make unapply-velerosa-role
# 	$(KUSTOMIZE) build config/default | kubectl delete -f -

build-deploy: THIS_IMAGE=ttl.sh/oadp-operator-$(shell git rev-parse --short HEAD):1h # Set target specific variable
build-deploy: ## Build current branch image and deploy controller to the k8s cluster specified in ~/.kube/config.
	IMG=$(THIS_IMAGE) make docker-build docker-push deploy

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

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

$(GOBIN)/yq:
	go install github.com/mikefarah/yq/v4@latest

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --extra-service-accounts "velero" --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	@make nullable-crds-bundle nullable-crds-config # patch nullables in CRDs
	# Copy updated bundle.Dockerfile to CI's Dockerfile.bundle
	# TODO: update CI to use generated one
	cp bundle.Dockerfile build/Dockerfile.bundle
	operator-sdk bundle validate ./bundle

.PHONY: nullable-crds-bundle
nullable-crds-bundle: DPA_SPEC_CONFIG_PROP = .spec.versions.0.schema.openAPIV3Schema.properties.spec.properties.configuration.properties
nullable-crds-bundle: PROP_RESOURCE_ALLOC = properties.podConfig.properties.resourceAllocations
nullable-crds-bundle: VELERO_RESOURCE_ALLOC = $(DPA_SPEC_CONFIG_PROP).velero.$(PROP_RESOURCE_ALLOC)
nullable-crds-bundle: RESTIC_RESOURCE_ALLOC = $(DPA_SPEC_CONFIG_PROP).restic.$(PROP_RESOURCE_ALLOC)
nullable-crds-bundle: DPA_CRD_YAML ?= bundle/manifests/oadp.openshift.io_dataprotectionapplications.yaml
nullable-crds-bundle: $(GOBIN)/yq
# Velero CRD
	@$(GOBIN)/yq '$(VELERO_RESOURCE_ALLOC).nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(VELERO_RESOURCE_ALLOC).properties.limits.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(VELERO_RESOURCE_ALLOC).properties.limits.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(VELERO_RESOURCE_ALLOC).properties.requests.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(VELERO_RESOURCE_ALLOC).properties.requests.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
# Restic CRD
	@$(GOBIN)/yq '$(RESTIC_RESOURCE_ALLOC).nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(RESTIC_RESOURCE_ALLOC).properties.limits.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(RESTIC_RESOURCE_ALLOC).properties.limits.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(RESTIC_RESOURCE_ALLOC).properties.requests.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)
	@$(GOBIN)/yq '$(RESTIC_RESOURCE_ALLOC).properties.requests.additionalProperties.nullable = true' $(DPA_CRD_YAML) > $(DPA_CRD_YAML).yqresult
	@mv $(DPA_CRD_YAML).yqresult $(DPA_CRD_YAML)

.PHONY: nullable-crds-config
nullable-crds-config: DPA_CRD_YAML ?= config/crd/bases/oadp.openshift.io_dataprotectionapplications.yaml
nullable-crds-config:
	DPA_CRD_YAML=$(DPA_CRD_YAML) make nullable-crds-bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) . $(DOCKER_BUILD_ARGS)

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

GIT_REV:=$(shell git rev-parse --short HEAD)
## Build current branch operator image, bundle image, push and install via OLM
.PHONY: deploy-olm
deploy-olm: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h # Set target specific variable
deploy-olm: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h # Set target specific variable
deploy-olm: DEPLOY_TMP:=$(shell mktemp -d)/ # Set target specific variable
deploy-olm:
	oc whoami # Check if logged in
	oc create namespace $(OADP_TEST_NAMESPACE) # This should error out if namespace already exists, delete namespace (to clear current resources) before proceeding
	@echo "DEPLOY_TMP: $(DEPLOY_TMP)"
	# build and push operator and bundle image
	# use operator-sdk to install bundle to authenticated cluster
	cp -r . $(DEPLOY_TMP) && cd $(DEPLOY_TMP) && \
	IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle bundle-build bundle-push; \
	rm -rf $(DEPLOY_TMP)
	operator-sdk run bundle $(THIS_BUNDLE_IMAGE) --namespace $(OADP_TEST_NAMESPACE)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
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
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# For testing oeprator upgrade
# opm upgrade
catalog-build-replaces: opm ## Build a catalog image using replace mode
	$(OPM) index add --container-tool docker --mode replaces --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

OADP_BUCKET = $(shell cat $(OADP_BUCKET_FILE))
TEST_FILTER := ($(shell echo '! aws && ! gcp && ! azure && ! ibmcloud' | \
sed -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")) || $(CLUSTER_TYPE)
#TEST_FILTER := $(shell echo '! aws && ! gcp && ! azure' | sed -r "s/[&]* [!] $(CLUSTER_TYPE)|[!] $(CLUSTER_TYPE) [&]*//")
SETTINGS_TMP=/tmp/test-settings

test-e2e-setup:
	mkdir -p $(SETTINGS_TMP)
	TARGET_CI_CRED_FILE="$(CI_CRED_FILE)" AZURE_RESOURCE_FILE="$(AZURE_RESOURCE_FILE)" CI_JSON_CRED_FILE="$(AZURE_CI_JSON_CRED_FILE)" \
	OADP_JSON_CRED_FILE="$(AZURE_OADP_JSON_CRED_FILE)" OADP_CRED_FILE="$(OADP_CRED_FILE)" OPENSHIFT_CI="$(OPENSHIFT_CI)" \
	PROVIDER="$(VELERO_PLUGIN)" BUCKET="$(OADP_BUCKET)" BSL_REGION="$(BSL_REGION)" SECRET="$(CREDS_SECRET_REF)" TMP_DIR=$(SETTINGS_TMP) \
	VSL_REGION="$(VSL_REGION)" BSL_AWS_PROFILE="$(BSL_AWS_PROFILE)" /bin/bash "tests/e2e/scripts/$(CLUSTER_TYPE)_settings.sh"

test-e2e: test-e2e-setup
	ginkgo run -mod=mod tests/e2e/ -- -credentials=$(OADP_CRED_FILE) \
	-velero_namespace=$(OADP_TEST_NAMESPACE) \
	-settings=$(SETTINGS_TMP)/oadpcreds \
	-velero_instance_name=$(VELERO_INSTANCE_NAME) \
	-timeout_multiplier=$(E2E_TIMEOUT_MULTIPLIER) \
	--ginkgo.label-filter="$(TEST_FILTER)" \
	-ci_cred_file=$(CI_CRED_FILE) \
	-provider=$(CLUSTER_TYPE) \
	-creds_secret_ref=$(CREDS_SECRET_REF) \
	-artifact_dir=$(ARTIFACT_DIR) \
	-oc_cli=$(OC_CLI)

test-e2e-cleanup:
	rm -rf $(SETTINGS_TMP)
