oc annotate hco kubevirt-hyperconverged -n openshift-cnv --overwrite kubevirt.kubevirt.io/jsonpatch='[
    {
      "op": "add",
      "path": "/spec/configuration/developerConfiguration/featureGates/-",
      "value": "IncrementalBackup"
    },
    {
      "op": "add",
      "path": "/spec/configuration/developerConfiguration/featureGates/-",
      "value": "UtilityVolumes"
    },
    {
      "op": "add",
      "path": "/spec/configuration/changedBlockTrackingLabelSelectors",
      "value": {
        "virtualMachineLabelSelector": {
          "matchLabels": {
            "changedBlockTracking": "true"
          }
        }
      }
    }
]'
