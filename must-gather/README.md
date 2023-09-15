# OADP must-gather

`must-gather` is a tool built on top of [OpenShift must-gather](https://github.com/openshift/must-gather)
that expands its capabilities to gather OADP operator specific resources

### Usage

**Full gather**
```sh
oc adm must-gather --image=quay.io/konveyor/oadp-must-gather:latest
```

The command above will create a local directory with a dump of the OADP Operator state.

You will get a dump of:
- All namespaces where OADP operator is installed, including pod logs
- All velero.io resources located in those namespaces
- Prometheus metrics

**Essential-only gather**

Differences from full gather:
 - Logs are only gathered from specified time window
 - Skips collection of prometheus metrics. Removes duplicate logs from payload.
```
# Essential gather (available time windows: [1h, 6h, 24h, 72h, all])
oc adm must-gather --image=quay.io/konveyor/oadp-must-gather:latest -- /usr/bin/gather_24h_essential
```

#### Preview metrics on local Prometheus server

Get Prometheus metrics data directory dump (last day, might take a while):
```sh
oc adm must-gather --image quay.io/konveyor/oadp-must-gather:latest -- /usr/bin/gather_metrics_dump
```

Run local Prometheus instance with dumped data:
```sh
make prometheus-run # and prometheus-cleanup when you're done
```
The latest Prometheus data file (prom_data.tar.gz) in current directory/subdirectories is searched by default. Could be specified in ```PROMETHEUS_DUMP_PATH``` environment variable.

### Known Limitations

`velero backups` with phase `FailedValidation` could cause `must-gather` to be slow. In order to speed up the process, pass a timeout value to the command as follows,
```sh
oc adm must-gather --image=quay.io/konveyor/oadp-must-gather:latest -- /usr/bin/gather_with_timeout <timeout_value_in_seconds>
```
### Support for insecure TLS connections 

If a custom CA cert is used, then must-gather pod fails to grab the output for velero logs/describe. In this case, the user can pass a flag to the must-gather command to allow insecure TLS connections. 

```sh
oc adm must-gather --image=quay.io/konveyor/oadp-must-gather:latest -- /usr/bin/gather_without_tls <true/false>
```
Note: By default the flag value is set to `false`

### Combining Options

It is not currently possible to combine multiple gather scripts, for example specifying a timeout value at the same time as allowing insecure TLS. However, it is possible in some cases to work around this limitation by setting internal variables on the must-gather command line, like this:

```sh
oc adm must-gather --image=quay.io/konveyor/oadp-must-gather:latest -- skip_tls=true /usr/bin/gather_with_timeout <timeout_value_in_seconds>
```

In this case, the `skip_tls` variable is set before running the gather_with_timeout script, and the net effect is a combination of gather_with_timeout and gather_without_tls. The only other variables that can be specified this way are `logs_since`, with a default value of `72h`, and `request_timeout`, with a default value of `0s`.

### Development
You can build the image locally using the Dockerfile included.

A `makefile` is also provided. To use it, you must pass a repository via the command-line using the variable `IMAGE_NAME`.
You can also specify the registry using the variable `IMAGE_REGISTRY` (default is [quay.io](https://quay.io)) and the tag via `IMAGE_TAG` (default is `latest`).

The targets for `make` are as follows:
- `build`: builds the image with the supplied name and pushes it
- `docker-build`: builds the image but does not push it
- `docker-push`: pushes an already-built image

For example:
```sh
make build IMAGE_NAME=my-repo/must-gather
```
would build the local repository as `quay.io/my-repo/must-gather:latest` and then push it.
