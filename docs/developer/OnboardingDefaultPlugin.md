# Onboarding new default plugin

api/v1alpha1/oadp_types.go
-  new `const <name> DefaultPlugin`
-  new `const <name> UnsupportedImageKey`

config/manager/manager.yaml
- new `spec.template.spec.containers["velero"].env` name/value pair to override default image

pkg/common/common.go
- new default plugin image url
- new default plugin name

pkg/credentials/credentials.go
- new `get<name>PluginImage` function
- add case to `getPluginImage` function
- add new plugin to map `PluginSpecificFields`

controllers/velero_test.go
Add tests for buildVeleroDeployment in check plugin exist in velero deployment as initContainer