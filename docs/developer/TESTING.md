<h1 align="center">E2E Testing</h1>

## Prerequisites:

### Install Ginkgo
```bash
$ go get -u github.com/onsi/ginkgo/ginkgo
```

### Setup backup storage configuration
To get started, the test suite expects 2 files to use as configuration for
Velero's backup storage. One file that contains your credentials, and another
that contains additional configuration options (for now the name of the
bucket).

The default test suite expects these files in `/var/run/oadp-credentials`, but
can be overridden with the environment variables `OADP_AWS_CRED_FILE` and
`OADP_S3_BUCKET`.

To get started, create these 2 files:
`OADP_AWS_CRED_FILE`:
```
aws_access_key_id=<access_key>
aws_secret_access_key=<secret_key>
```

`OADP_S3_BUCKET`:
```json
{
  "velero-bucket-name": <bucket>
}
```

## Run all e2e tests
```bash
$ make test-e2e
```
## Run selected test
You can run a particular e2e test(s) by placing an `F` at the beginning of a
`Describe`, `Context`, and `It`. 

```
 FDescribe("test description", func() { ... })
 FContext("test scenario", func() { ... })
 FIt("the assertion", func() { ... })
```

These need to be removed to run all specs.

