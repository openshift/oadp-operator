.PHONY:ginkgo
ginkgo: # Make sure ginkgo is in $GOPATH/bin
	go get github.com/onsi/ginkgo/ginkgo
	go get github.com/onsi/gomega/...

test-e2e:
	ginkgo tests/e2e/
