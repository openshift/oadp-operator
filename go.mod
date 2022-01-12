module github.com/openshift/oadp-operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.28.2
	github.com/go-logr/logr v1.2.0
	github.com/google/uuid v1.1.2
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.0.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.1.1
	github.com/onsi/gomega v1.18.1
	github.com/openshift/api v0.0.0-20211209135129-c58d9f695577
	github.com/openshift/client-go v0.0.0-20211209144617-7385dd6338e3
	github.com/operator-framework/api v0.10.7
	github.com/operator-framework/operator-lib v0.9.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.51.2
	github.com/vmware-tanzu/velero v1.7.0 // TODO: Update this to a pinned version
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	k8s.io/api v0.23.0
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.23.0
	k8s.io/client-go v0.23.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/controller-runtime v0.10.3
)

replace (
	github.com/go-logr/logr v1.2.0 => github.com/go-logr/logr v0.4.0
	github.com/openshift/api v0.0.0-20211209135129-c58d9f695577 => github.com/openshift/api v0.0.0-20210805075156-d8fab4513288
	k8s.io/api v0.23.0 => k8s.io/api v0.22.2
	k8s.io/apimachinery v0.23.0 => k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.23.0 => k8s.io/client-go v0.22.2
	k8s.io/klog/v2 v2.30.0 => k8s.io/klog/v2 v2.9.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b => k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a

)
