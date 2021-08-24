# E2E Testing

## Prerequisites

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

## Run Tests
```bash
$ make test-e2e
```
