# Test OADP version upgrade

To test an upgrade from a version to another, run
```sh
make catalog-test-upgrade
```
from the branch you want to test the upgrade (master or a release branch).

This will create `oadp-operator-catalog-test-upgrade` catalog in your cluster (OperatorHub in the UI).

![catalog in OperatorHUB](../../images/test_oadp_version_upgrade_catalog.png)

The catalog will have two channels:

- the previous version (the released version prior to the version of the branch you are)

    this is defined in `PREVIOUS_CHANNEL` variable in Makefile, and can be cahnged

- and the current version (the version of the branch you are, which can be yet unreleased, if on master branch)

Select the new catalog in the UI and install selecting the previous version (for example, `stable 1.2`) update channel. This will install OADP operator from that branch (in the example, `oadp-1.2` branch).

![Channel selecting](../../images/test_oadp_version_upgrade_channel.png)

Finally, after  creating a DPA, change channel of the installed operator to the next version (for example, `stable` - version 99.0.0, which points to master branch) in Subscription tab.

![Update subscription channel](../../images/test_oadp_version_upgrade_subscription.png)

To delete `oadp-operator-catalog-test-upgrade` catalog in your cluster after tests:
- uninstall operator and run `oc delete catalogsource oadp-operator-catalog-test-upgrade -n openshift-marketplace`
- or run `make undeploy-olm`
