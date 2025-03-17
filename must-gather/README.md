# oadp-must-gather

refactor of OADP's must-gather

```shell
# fast test of must gather
go run cmd/main.go

# real test of must-gather
podman build -t ttl.sh/oadp/must-gather-$(git rev-parse --short HEAD)-$(echo $RANDOM) -f Dockerfile . --platform=<cluster-architecture>
podman push <this-image>
oc adm must-gather --image=<this-image> -- /usr/bin/gather -h
oc adm must-gather --image=<this-image>
# TODO test omg https://github.com/openshift/oadp-operator/pull/1269
```

TODO write contributing and pre-requisites

TODO when to update go.mod dependencies (like velero, oadp, nac)
```
go get github.com/openshift/oadp-operator@master
go get github.com/migtools/oadp-non-admin@master
go get github.com/openshift/oc@release-4.17
```
