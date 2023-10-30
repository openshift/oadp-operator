# Potentially helpful debug commands

## DataMover

Compile `getDataMoverResources` script
```
go build getDataMoverResources.go
```

> **Note:** if you are using VSM/Volsync DataMover (OADP 1.2 or lower), pass the flag `-v=false` to the next commands.

### watch backup resources live

```
watch -n 10 ./getDataMoverResources -b -d
```

### watch restore resources live

```
watch -n 10 ./getDataMoverResources -r -d
```
