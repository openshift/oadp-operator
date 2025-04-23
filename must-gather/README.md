# OADP Must-gather

To test OADP Must-gather, run
```sh
go run cmd/main.go -h
go run cmd/main.go
```

To test OADP Must-gather with `oc adm must-gather`, run
```sh
podman build -t ttl.sh/oadp/must-gather-$(git rev-parse --short HEAD)-$(echo $RANDOM):1h -f Dockerfile . --platform=<cluster-architecture>
podman push <this-image>
oc adm must-gather --image=<this-image> -- /usr/bin/gather -h
oc adm must-gather --image=<this-image>
```
OADP Must-gather is also tested through OADP E2E tests, being run after test cases and checking if summary does not contain errors and all required objects were collected.

To test omg tool, create `omg.Dockerfile` file
```Dockerfile
FROM python:3.10.12-slim-bullseye

WORKDIR /test-omg
COPY ./ ./
RUN pip install o-must-gather
```
and run
```sh
podman build -t omg-container -f omg.Dockerfile .
podman run -ti --rm omg-container bash
# inside container
omg use must-gather/clusters/
omg get backup -n <namespace> # and other OADP resources
```

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

Prior to each release, OADP Must-gather must be updated.

To update OADP Must-gather `go.mod` dependencies, run
```sh
go get github.com/openshift/oadp-operator@<release-brach>
go get github.com/migtools/oadp-non-admin@<release-brach>
# manually update github.com/openshift/velero version in replace section of go.mod to match OADP operator
go mod tidy
go mod verify
```

`must-gather/pkg/cli.go` file must be updated
```diff
 const (
-	mustGatherVersion = "1.5.0"
+	mustGatherVersion = "1.5.1"
	mustGatherImage   = "registry.redhat.io/oadp/oadp-mustgather-rhel9:v1.5"
```

> **Note:** If it is a minor release, `mustGatherImage` must also be updated.

## Deprecated folder

Scripts under `deprecated/` folder are for backwards compatibility with old OADP Must-gather shell script. Users should use new OADP Must-gather Go script, as highlighted in product documentation.
