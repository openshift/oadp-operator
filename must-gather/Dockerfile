FROM --platform=$BUILDPLATFORM quay.io/konveyor/builder:ubi9-latest AS konveyor-builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG RESTIC_BRANCH=konveyor-0.15.0
ARG VELERO_BRANCH=konveyor-dev
WORKDIR /build
RUN curl --location --output velero.tgz https://github.com/openshift/velero/archive/refs/heads/${VELERO_BRANCH}.tar.gz && \
    tar -xzvf velero.tgz && cd velero-${VELERO_BRANCH} && \
    VELERO_COMMIT=$(git ls-remote https://github.com/openshift/velero HEAD | awk '{printf $1}') && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -mod=mod -ldflags '-extldflags "-static" -X github.com/vmware-tanzu/velero/pkg/buildinfo.Version='"${VELERO_BRANCH}"' -X github.com/vmware-tanzu/velero/pkg/buildinfo.GitSHA='"${VELERO_COMMIT}" -o /velero github.com/vmware-tanzu/velero/cmd/velero && \
    cd .. && rm -rf velero.tgz velero-${VELERO_BRANCH} && \
    curl --location --output restic.tgz https://github.com/openshift/restic/archive/refs/heads/${RESTIC_BRANCH}.tar.gz && \
    tar -xzvf restic.tgz && cd restic-${RESTIC_BRANCH} && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -mod=mod -ldflags '-extldflags "-static"' -o /restic github.com/restic/restic/cmd/restic && \
    cd .. && rm -rf restic.tgz restic-${RESTIC_BRANCH}

FROM registry.access.redhat.com/ubi9/go-toolset:latest AS gobuilder

RUN go install -v github.com/google/pprof@latest

FROM quay.io/openshift/origin-must-gather:latest AS builder

FROM registry.access.redhat.com/ubi9-minimal:latest

RUN echo -ne "[centos-9-appstream]\nname = CentOS 9 (RPMs) - AppStream\nbaseurl = https://mirror.stream.centos.org/9-stream/AppStream/x86_64/os/\nenabled = 1\ngpgcheck = 0" > /etc/yum.repos.d/centos-9-appstream.repo
RUN microdnf -y install rsync tar gzip graphviz findutils

COPY --from=gobuilder /opt/app-root/src/go/bin/pprof /usr/bin/pprof
COPY --from=builder /usr/bin/oc /usr/bin/oc
COPY --from=konveyor-builder /velero /usr/bin/velero
COPY --from=konveyor-builder /restic /usr/bin/restic

COPY collection-scripts/* /usr/bin/
COPY collection-scripts/debug/* /usr/bin/
COPY collection-scripts/logs/* /usr/bin/
COPY collection-scripts/time_window_gather /usr/bin/

ENTRYPOINT /usr/bin/gather
