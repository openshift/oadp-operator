module github.com/openshift/oadp-operator

go 1.16

require (
	cloud.google.com/go/storage v1.10.0
	github.com/aws/aws-sdk-go v1.28.2
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/ginkgo/v2 v2.1.1
	github.com/onsi/gomega v1.17.0
	github.com/openshift/api v0.0.0-20210805075156-d8fab4513288
	github.com/operator-framework/api v0.10.7
	github.com/operator-framework/operator-lib v0.9.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.51.2
	github.com/vmware-tanzu/velero v1.7.0 // TODO: Update this to a pinned version
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/api v0.44.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.10.3
)
