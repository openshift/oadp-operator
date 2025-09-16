FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.24 AS builder
COPY . /workspace
WORKDIR /workspace
ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 GOOS=linux go build -mod=mod -a -tags strictfipsruntime -o /workspace/bin/manager main.go

FROM registry.redhat.io/ubi9/ubi:latest
RUN dnf -y install openssl && dnf -y reinstall tzdata && dnf clean all
WORKDIR /
COPY --from=builder /workspace/bin/manager .
COPY --from=builder /workspace/LICENSE /licenses/
USER 65532:65532
ENTRYPOINT ["/manager"]
