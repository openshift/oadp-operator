FROM registry.access.redhat.com/ubi8/go-toolset:1.14.7 as gobuilder

RUN go get github.com/google/pprof

FROM quay.io/openshift/origin-must-gather:4.7 as builder

FROM registry.access.redhat.com/ubi8-minimal:latest
RUN echo -ne "[centos-8-appstream]\nname = CentOS 8 (RPMs) - AppStream\nbaseurl = http://mirror.centos.org/centos-8/8/AppStream/x86_64/os/\nenabled = 1\ngpgcheck = 0" > /etc/yum.repos.d/centos.repo

RUN microdnf -y install rsync tar gzip graphviz findutils

COPY --from=gobuilder /opt/app-root/src/go/bin/pprof /usr/bin/pprof
COPY --from=builder /usr/bin/oc /usr/bin/oc

COPY collection-scripts/* /usr/bin/
COPY collection-scripts/logs/* /usr/bin/
COPY collection-scripts/time_window_gather /usr/bin/

ENTRYPOINT /usr/bin/gather
