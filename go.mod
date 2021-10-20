module github.com/openshift/oadp-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/openshift/api v0.0.0-20210805075156-d8fab4513288
	github.com/operator-framework/operator-lib v0.9.0
	github.com/vmware-tanzu/velero v1.7.0 // TODO: Update this to a pinned version
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/vmware-tanzu/velero v1.6.1-0.20210806003158-ed5809b7fc22 // TODO: Update this to a pinned version
	k8s.io/api v0.22.0
	k8s.io/apiextensions-apiserver v0.21.3
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v0.22.0
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.5
)
