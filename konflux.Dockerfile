FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.23 AS builder
COPY . /workspace
WORKDIR /workspace
ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 GOOS=linux go build -mod=mod -a -tags strictfipsruntime -o /workspace/bin/manager cmd/main.go

#FROM registry.redhat.io/ubi9/ubi-minimal:latest
FROM brew.registry.redhat.io/rh-osbs/openshift-openshift-enterprise-base-rhel9@sha256:f0a0b69511af8b8e6555ccede3ea2ab965a7f707e6e13f480a86aeb67d86ab99
RUN dnf -y install openssl && dnf -y reinstall tzdata && dnf clean all
WORKDIR /
COPY --from=builder /workspace/bin/manager .
COPY LICENSE /licenses/
USER 65532:65532
ENTRYPOINT ["/manager"]
