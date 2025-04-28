FROM --platform=$BUILDPLATFORM quay.io/konveyor/builder:ubi9-latest AS builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG KOPIA_BRANCH=oadp-1.4
ARG RESTIC_BRANCH=oadp-1.4

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY cmd/main.go cmd/main.go
COPY pkg/ pkg/
COPY deprecated/ deprecated/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -mod=mod -a -o gather cmd/main.go

RUN curl --location --output kopia.tgz https://github.com/migtools/kopia/archive/refs/heads/${KOPIA_BRANCH}.tar.gz && \
    tar -xzvf kopia.tgz && cd kopia-${KOPIA_BRANCH} && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -mod=mod -a -ldflags '-extldflags "-static"' -o /kopia github.com/kopia/kopia && \
    cd .. && rm -rf kopia.tgz kopia-${KOPIA_BRANCH}

RUN curl --location --output restic.tgz https://github.com/openshift/restic/archive/refs/heads/${RESTIC_BRANCH}.tar.gz && \
    tar -xzvf restic.tgz && cd restic-${RESTIC_BRANCH} && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -mod=mod -a -ldflags '-extldflags "-static"' -o /restic github.com/restic/restic/cmd/restic && \
    cd .. && rm -rf restic.tgz restic-${RESTIC_BRANCH}

FROM registry.access.redhat.com/ubi9-minimal:latest

# oc adm must-gather uses these packages to download the output
RUN microdnf -y install rsync tar

COPY --from=builder /workspace/gather /usr/bin/gather
COPY --from=builder /workspace/deprecated/* /usr/bin/
COPY --from=builder /kopia /usr/bin/kopia
COPY --from=builder /restic /usr/bin/restic

ENTRYPOINT ["/usr/bin/gather"]
