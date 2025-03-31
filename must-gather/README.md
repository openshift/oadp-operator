# OADP Must-gather

To test OADP Must-gather, run
```sh
go run cmd/main.go -h
go run cmd/main.go
```

To test OADP Must-gather with `oc adm must-gather`, run
```sh
podman build -t ttl.sh/oadp/must-gather-$(git rev-parse --short HEAD)-$(echo $RANDOM) -f Dockerfile . --platform=<cluster-architecture>
podman push <this-image>
oc adm must-gather --image=<this-image> -- /usr/bin/gather -h
oc adm must-gather --image=<this-image>
```
TODO mention e2e tests

TODO test omg https://github.com/openshift/oadp-operator/pull/1269

To update OADP Must-gather `go.mod` dependencies, run
```sh
go get github.com/openshift/oadp-operator@master
go get github.com/migtools/oadp-non-admin@master
# manually update github.com/openshift/velero version in replace section of go.mod to match OADP operator
go mod tidy
go mod verify
```
Update it often. It must be updated prior to releases.

Possible necessary updates over the time
```sh
go get github.com/openshift/oc@<branch-or-commit>
go mod tidy
go mod verify
```

## OADP release

```sh
go get github.com/openshift/oadp-operator@<release-brach>
go get github.com/migtools/oadp-non-admin@<release-brach>
# manually update github.com/openshift/velero version in replace section of go.mod to match OADP operator
go mod tidy
go mod verify
```
