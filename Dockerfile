# Build the manager binary
# TODO! Find a real ubi8 image for golang 1.16
# FROM quay.io/app-sre/boilerplate:image-v2.1.0 as builder
FROM quay.io/konveyor/builder as builder

WORKDIR /go/src/github.com/openshift/oadp-operator
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY pkg/ pkg/
COPY controllers/ controllers/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -a -o /go/src/manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8-minimal
WORKDIR /
COPY --from=builder /go/src/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
