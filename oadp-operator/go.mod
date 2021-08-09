module github.com/openshift/oadp-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20210729133136-d870cea76006
	github.com/prometheus/common v0.26.0 // indirect
	github.com/vmware-tanzu/velero v1.6.1-0.20210806003158-ed5809b7fc22
	golang.org/x/tools v0.1.2 // indirect
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2 // indirect
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.2
)
