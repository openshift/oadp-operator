# Potentially helpful debug commands

## watch DataMover backup resources live
```
watch -n 10 go run getDataMoverResources.go -b -d
```

## watch DataMover restore resources live
```
watch -n 10 go run getDataMoverResources.go -r -d
```