# Watching Secrets

The provider secret gets created by user, and it is not part of the operator lifecyle itself. Inorder for the DPA to update the current state incase if the provider secrets gets deleted or updated, the secret object needs to be watched as a part of the reconcile loop. To achieve this, the secrets are labeled with the following,

```
 1. oadpApi.OadpOperatorLabel: "True"
 2. <namespace>.dataprotectionapplication: <name>
```

where <namespace> is the namespace where OADP operator is installed and <name> is the name of the DPA instance

# Current State

When the labeled secret objects gets deleted, the operator status gets updated accordingly. Once that happens, if a new secret gets created in the place of original secret, it does not get labeled as of now. There are plans in the future to automatically label the incoming secrets and add it to the reconcile loop. For now, inorder to trigger the DPA CR status update, we suggest recreating the operator pod. 
