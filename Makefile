OADP_TEST_NAMESPACE ?= oadp-operator-e2e
CREDS_SECRET_REF ?= cloud-credentials
OADP_AWS_CRED_FILE ?= /var/run/oadp-credentials/aws-credentials
OADP_S3_BUCKET ?= /var/run/oadp-credentials/velero-bucket-name
VELERO_INSTANCE_NAME ?= example-velero

.PHONY:ginkgo
ginkgo: # Make sure ginkgo is in $GOPATH/bin
	go get github.com/onsi/ginkgo/ginkgo
	go get github.com/onsi/gomega/...

test-e2e:
	ginkgo tests/e2e/ -- -cloud=$(OADP_AWS_CRED_FILE) \
	-s3_bucket=$(OADP_S3_BUCKET) -velero_namespace=$(OADP_TEST_NAMESPACE) \
	-creds_secret_ref=$(CREDS_SECRET_REF) \
	-velero_instance_name=$(VELERO_INSTANCE_NAME)
