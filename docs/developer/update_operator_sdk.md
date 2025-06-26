## Upgrade Operator SDK version

To upgrade Operator SDK version, create Operator SDK structure using the current Operator SDK version and the upgrade version (get Operator SDK executables in https://github.com/operator-framework/operator-sdk/releases), using the same commands used to scaffold the project, in two different folders.

The project was generated using Operator SDK version v1.35.0, running the following commands
```sh
operator-sdk init \
  --project-name=oadp-operator \
  --repo=github.com/openshift/oadp-operator \
  --domain=openshift.io
operator-sdk create api \
  --group oadp \
  --version v1alpha1 \
  --kind DataProtectionApplication \
  --resource --controller
operator-sdk create api \
  --group oadp \
  --version v1alpha1 \
  --kind CloudStorage \
  --resource --controller
```
> **NOTE:** The information about plugin and project version, as well as project name, repo and domain, is stored in [PROJECT](../../PROJECT) file

Then generate a `diff` file from the two folders and apply changes to project code.

Example
```sh
mkdir current
mkdir new
cd current
# Run Operator SDK commands pointing to Operator SDK executable with the current version
cd ..
cd new
# Run Operator SDK commands pointing to Operator SDK executable with the new version
cd ..
diff -ruN current new > operator-sdk-upgrade.diff
patch -p1 --verbose -d ./ -i operator-sdk-upgrade.diff
# Resolve possible conflicts
```