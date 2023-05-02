FROM quay.io/konveyor/builder:latest as konveyor-builder
ARG RESTIC_BRANCH=konveyor-0.15.0
ARG VELERO_BRANCH=konveyor-dev
WORKDIR /build
RUN curl --location --output velero.tgz https://github.com/openshift/velero/archive/refs/heads/${VELERO_BRANCH}.tar.gz && \
    tar -xzvf velero.tgz && cd velero-${VELERO_BRANCH} && \
    CGO_ENABLED=0 GOOS=linux go build -a -mod=mod -ldflags '-extldflags "-static" -X github.com/vmware-tanzu/velero/pkg/buildinfo.Version=${VELERO_BRANCH}' -o /velero github.com/vmware-tanzu/velero/cmd/velero && \
    cd .. && rm -rf velero.tgz velero-${VELERO_BRANCH} && \
    curl --location --output restic.tgz https://github.com/openshift/restic/archive/refs/heads/${RESTIC_BRANCH}.tar.gz && \
    tar -xzvf restic.tgz && cd restic-${RESTIC_BRANCH} && \
    CGO_ENABLED=0 GOOS=linux go build -a -mod=mod -ldflags '-extldflags "-static"' -o /restic github.com/restic/restic/cmd/restic && \
    cd .. && rm -rf restic.tgz restic-${RESTIC_BRANCH}

FROM registry.access.redhat.com/ubi8/go-toolset:1.17.10 as gobuilder

RUN go install -v github.com/google/pprof@latest

FROM quay.io/openshift/origin-must-gather:4.10 as builder

FROM registry.access.redhat.com/ubi8-minimal:latest

RUN microdnf -y install rsync tar gzip graphviz findutils

COPY --from=gobuilder /opt/app-root/src/go/bin/pprof /usr/bin/pprof
COPY --from=builder /usr/bin/oc /usr/bin/oc
COPY --from=konveyor-builder /velero /usr/bin/velero
COPY --from=konveyor-builder /restic /usr/bin/restic

COPY collection-scripts/* /usr/bin/
COPY collection-scripts/logs/* /usr/bin/
COPY collection-scripts/time_window_gather /usr/bin/

ENTRYPOINT /usr/bin/gather
