# OADP Must-gather

## New Markdown UI for summarized results

In order to better facilitate the quick analsysis of customer issues, OADP now offers a markown summary of the collected information.
The `oadp-must-gather-summary.md` file can be found under the clusters directory in the oadp-must-gather results.

#### Example screenshot #1
![Screenshot from 2025-05-05 10-33-47](https://github.com/user-attachments/assets/16f4933f-513b-4ce5-b128-0c89aafedc7e)

#### Example screenshot #2
![Screenshot from 2025-05-05 10-34-05](https://github.com/user-attachments/assets/fd8c6205-dadc-4bcb-852d-ecfd5ea81cff)

## Developer Setup
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
go get github.com/openshift/oadp-operator@oadp-dev
go get github.com/migtools/oadp-non-admin@oadp-dev
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

## Standards

OADP Must-gather must comply with https://github.com/openshift/enhancements/blob/995b620cb04c030bf62c908e188472fe7031a704/enhancements/oc/must-gather.md?plain=1#L94-L104

>1. Must have a zero-arg, executable file at `/usr/bin/gather` that does your default gathering

OADP Must-gather binary can be called without args. All OADP Must-gather binary args are optional

>2. Must produce data to be copied back at `/must-gather`. The data must not contain any sensitive data. We don't string PII information, only secret information.

OADP Must-gather collected data is stored at `/must-gather` folder in the same path the binary was called.

Most of the data is collected through `oc adm inspect` command (including Secrets). The other data are cluster information, OADP related information (CRDs and CRs) and storage information (StorageClasses, VolumeSnapshotClasses and CSIDrivers CRDs and CRs). These objects should not contain any sensitive data.

>3. Must produce a text `/must-gather/version` that indicates the product (first line) and the version (second line, `major.minor.micro-qualifier`),
>   so that programmatic analysis can be developed.

OADP Must-gather stores version information in `/must-gather/version` file

Example content of the file
```txt
OpenShift API for Data Protection (OADP) Must-gather
oadp-dev-branch
```

>4. Should honor the user-provided values for `--since` and `--since-time`, which are passed to plugins via
>   environment variables named `MUST_GATHER_SINCE` and `MUST_GATHER_SINCE_TIME`, respectively.

TODO `oc adm inspect` command is called through Go code. But both `since` and `since-time` are private. Need to change this in https://github.com/openshift/oc/blob/ae1bd9e4a75b8ab617a569e5c8e1a0d7285a16f6/pkg/cli/admin/inspect/inspect.go#L118-L119 to allow usage in OADP Must-gather
