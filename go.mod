module github.com/openshift/oadp-operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.34.11
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.2.0
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.1.1
	github.com/onsi/gomega v1.17.0
	github.com/openshift/api v0.0.0-20210805075156-d8fab4513288
	github.com/operator-framework/api v0.10.7
	github.com/operator-framework/operator-lib v0.9.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.51.2
	github.com/sirupsen/logrus v1.8.1
	github.com/vmware-tanzu/velero v1.8.0 // TODO: Update this to a pinned version
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d // indirect
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/controller-runtime v0.10.3
)

replace github.com/vmware-tanzu/velero => github.com/konveyor/velero v0.10.2-0.20220608184903-e0a380fc0705
