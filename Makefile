OADP_TEST_NAMESPACE ?= openshift-adp
REGION ?= us-east-1
PROVIDER ?= aws
CLUSTER_PROFILE ?= aws
CREDS_SECRET_REF ?= cloud-credentials
OADP_AWS_CRED_FILE ?= /var/run/oadp-credentials/aws-credentials
OADP_S3_BUCKET ?= /var/run/oadp-credentials/velero-bucket-name
VELERO_INSTANCE_NAME ?= velero-sample
E2E_TIMEOUT_MULTIPLIER ?= 1

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

.PHONY:ginkgo
ginkgo: # Make sure ginkgo is in $GOPATH/bin
	go get github.com/onsi/ginkgo/ginkgo
	go get github.com/onsi/gomega/...

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 99.0.0

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
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -mod=mod ./controllers/... ./pkg/... -coverprofile cover.out


ENVTEST = $(shell pwd)/bin/setup-envtest
ci-test: ## This assumes "manifests generate fmt vet envtest" ran.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -mod=mod ./controllers/... -coverprofile cover.out


##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

DEPLOY_TMP=/tmp/oadp-make-deploy
deploy-tmp: kustomize
	mkdir -p $(DEPLOY_TMP)
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-service_account.yaml > $(DEPLOY_TMP)/velero-service_account.yaml
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-role.yaml > $(DEPLOY_TMP)/velero-role.yaml
	sed -e 's/namespace: system/namespace: $(OADP_TEST_NAMESPACE)/g' config/velero/velero-role_binding.yaml > $(DEPLOY_TMP)/velero-role_binding.yaml
deploy-tmp-cleanup:
	rm -rf $(DEPLOY_TMP)
deploy: manifests deploy-tmp  ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	kubectl apply -f $(DEPLOY_TMP)/velero-service_account.yaml
	kubectl apply -f $(DEPLOY_TMP)/velero-role.yaml
	kubectl apply -f $(DEPLOY_TMP)/velero-role_binding.yaml
	make deploy-tmp-cleanup

undeploy: deploy-tmp ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f $(DEPLOY_TMP)/velero-service_account.yaml
	kubectl delete -f $(DEPLOY_TMP)/velero-role.yaml
	kubectl delete -f $(DEPLOY_TMP)/velero-role_binding.yaml
	$(KUSTOMIZE) build config/default | kubectl delete -f -
	make deploy-tmp-cleanup

build-deploy: THIS_IMAGE=ttl.sh/oadp-operator-$(shell git rev-parse --short HEAD):1h # Set target specific variable
build-deploy: ## Build current branch image and deploy controller to the k8s cluster specified in ~/.kube/config.
	IMG=$(THIS_IMAGE) make docker-build docker-push deploy

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

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

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	# Removes placeholder velero deployment used so `operator-sdk generate bundle` adds velero service account and role to bundle CSV using yq. See https://github.com/mikefarah/yq/#install
	yq eval 'del(.spec.install.spec.deployments.1)' bundle/manifests/oadp-operator.clusterserviceversion.yaml > bundle/manifests/oadp-operator.clusterserviceversion.yaml.yqresult
	mv bundle/manifests/oadp-operator.clusterserviceversion.yaml.yqresult bundle/manifests/oadp-operator.clusterserviceversion.yaml
	# Copy updated bundle.Dockerfile to CI's Dockerfile.bundle
	# TODO: update CI to use generated one
	cp bundle.Dockerfile build/Dockerfile.bundle
	operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

## Build current branch operator image, bundle image, push and install via OLM
deploy-olm: GIT_REV=$(shell git rev-parse --short HEAD)
deploy-olm: THIS_OPERATOR_IMAGE?=ttl.sh/oadp-operator-$(GIT_REV):1h # Set target specific variable
deploy-olm: THIS_BUNDLE_IMAGE?=ttl.sh/oadp-operator-bundle-$(GIT_REV):1h # Set target specific variable
deploy-olm:
	oc whoami # Check if logged in
	oc create namespace $(OADP_TEST_NAMESPACE) # This should error out if namespace already exists, delete namespace (to clear current resources) before proceeding
	IMG=$(THIS_OPERATOR_IMAGE) BUNDLE_IMG=$(THIS_BUNDLE_IMAGE) \
		make docker-build docker-push bundle bundle-build bundle-push # build and push operator and bundle image
	operator-sdk run bundle $(THIS_BUNDLE_IMAGE) --namespace $(OADP_TEST_NAMESPACE) # use operator-sdk to install bundle to authenticated cluster

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

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

S3_BUCKET := $(shell cat $(OADP_S3_BUCKET) | awk '/velero-bucket-name/  {gsub(/"/, "", $$2); print $$2}')
SETTINGS_TMP=/tmp/test-settings
test-e2e:
	mkdir -p $(SETTINGS_TMP)
	PROVIDER="$(PROVIDER)" BUCKET="$(S3_BUCKET)" REGION="$(REGION)" SECRET="$(CREDS_SECRET_REF)" TMP_DIR=$(SETTINGS_TMP) /bin/bash tests/e2e/scripts/aws_settings.sh
	ginkgo -mod=mod tests/e2e/ -- -cloud=$(OADP_AWS_CRED_FILE) \
	-velero_namespace=$(OADP_TEST_NAMESPACE) \
	-settings=$(SETTINGS_TMP)/awscreds \
	-velero_instance_name=$(VELERO_INSTANCE_NAME) \
	-timeout_multiplier=$(E2E_TIMEOUT_MULTIPLIER) \
	-cluster_profile=$(CLUSTER_PROFILE)
test-e2e-cleanup:
	rm -rf $(SETTINGS_TMP)
