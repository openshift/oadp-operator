# This Dockerfile WILL used by PROW CI to run E2E tests of the repo
FROM quay.io/konveyor/builder AS builder

WORKDIR /go/src/github.com/openshift/oadp-operator

COPY ./ .

RUN go mod download && \
    mkdir -p $(go env GOCACHE) && \
    chmod -R 777 ./ $(go env GOCACHE) $(go env GOPATH)
