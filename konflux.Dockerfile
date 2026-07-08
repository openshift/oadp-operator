FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.26 AS builder
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

LABEL description="OpenShift API for Data Protection - Operator"
LABEL io.k8s.description="OpenShift API for Data Protection - Operator"
LABEL io.k8s.display-name="OADP Operator"
LABEL io.openshift.tags="migration"
LABEL summary="OpenShift API for Data Protection - Operator"
