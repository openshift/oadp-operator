FROM registry.access.redhat.com/ubi8/go-toolset:1.17.10 as gobuilder

RUN go install -v github.com/google/pprof@latest

FROM quay.io/openshift/origin-must-gather:4.10 as builder

FROM registry.access.redhat.com/ubi8-minimal:latest

RUN microdnf -y install rsync tar gzip graphviz findutils

COPY --from=gobuilder /opt/app-root/src/go/bin/pprof /usr/bin/pprof
COPY --from=builder /usr/bin/oc /usr/bin/oc

COPY collection-scripts/* /usr/bin/
COPY collection-scripts/logs/* /usr/bin/
COPY collection-scripts/time_window_gather /usr/bin/

ENTRYPOINT /usr/bin/gather
