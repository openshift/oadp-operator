# Unsupported Arguments schema and common design
Date: 2024-05-29

## Abstract
OADP allows to configure it's components parameters via number of DataProtectionApplication Schema.

Unsupported arguments within the OADP operator are those that do not conform to the standard DataProtectionApplication Schema or are explicitly identified as unsupported or custom arguments.

There is need to provide a common way for defining and managing unsupported arguments within the OADP operator for its' components such as velero server or node-agent arguments.

## Background
The OADP allows configuration of several predefined arguments via the DataProtectionApplication Schema. These parameters are carefully selected and tested to meet the highest quality standards. However, in some circumstances, there is a need to pass other parameters that may be under development, not yet fully tested, or specifically overriding the default ones.

Over the past OADP releases, some implementations have provided access to extra arguments. However, there has been no standardization, and unknown to DataProtectionApplication Schema arguments were not allowed.

## Goals
- Define common way to allow passing extra arguments to various executables used by OADP.
- Ensure the new common way do not interfere with already existing implementations.

## Non Goals
- Deprecate current ways of defining extra arguments.
- Change precedences within current ways of defining extra arguments.
- Validation of unsupported args, they are passed to the appropriate executables without any prior validation.

## Design
Unsupported arguments are defined as a `ConfigMap` within the same Namespace as OADP Operator.

Multiple `ConfigMaps` may exist to represent specific sections of the OADP configuration, such as dedicated `ConfigMap` instances for Velero server arguments or node-agent arguments.

The user must provide `DPA` annotation(s) with the `name` of the `ConfigMap` where the OADP configuration for the unsupported arguments is present.

When ConfigMap is found, controller takes those values as the highest priority and replaces **all** of the other configuration options, even if they were defined within `DPA` schema for the section for which the `ConfigMap` was created.

The Schema of an `ConfigMap` includes data that corresponds to the arguments passed to the relevant executable as in the below example:

<a name="notes"></a>
> ℹ️ **Note 1:** If an argument name is passed without `-` prefix or `--` prefix, then `--` is added to the argument name.

> ℹ️ **Note 2:** A boolean argument value `true` or `false` is always converted to lower-case.

> ℹ️ **Note 3:** An argument value is always combined with the argument name using `=` character in between.


  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: oadp-unsupported-velero-server-args
    namespace: openshift-adp
  data:
    default-volume-snapshot-locations: aws:backups-primary,azure:backups-secondary
    log-level: debug
    default-snapshot-move-data: true
  ```

Above example will translate to the `velero` args:

```shell
velero server --default-volume-snapshot-locations=aws:backups-primary,azure:backups-secondary --log-level=debug --default-snapshot-move-data=true
```

The user has to create DPA with the annotation pointing to a relevant ConfigMap as in the example where both `oadp-unsupported-velero-server-args` and `unsupported-node-agent` ConfigMaps are expected to be present:

  ```yaml
  kind: DataProtectionApplication
  apiVersion: oadp.openshift.io/v1alpha1
  metadata:
  name: sample-dpa
  namespace: openshift-adp
  annotations:
      oadp.openshift.io/unsupported-velero-server-args: 'oadp-unsupported-velero-server-args'
      oadp.openshift.io/unsupported-node-agent-args: 'unsupported-node-agent'
  spec:
      [...]
  ```

## Alternatives Considered
 - Resource specific configuration / CRD options
 
   The configuration data is directly integrated into the Custom Resource Definition (CRD). This is how currently some of the extra arguments are defined within OADP.

   There is however schema enforcement, which is problematic, because it requires new release of OADP to include unknown or not available options.
   
   This approach also introduces more complexity as it requires logic to handle and validate each of the options. On top of that having CRD options introduces migration and upgrade problems requiring migration strategies to be taken into account for existing resources. 

   Adding the unsupported arguments and options within CRD makes those options highly visible to the users and may encourage to use them.

 - Annotations to include unsupported args directly

   This method allows easily to add args to existing resources, however they have size limitations, restricting the amount of data that can be stored and may have impact on the scenarios where limit is reached.

   Annotations are also problematic to maintain when used for more complex types of configurations.

   Sample custom unsupported args:

    ```yaml
    kind: DataProtectionApplication
    apiVersion: oadp.openshift.io/v1alpha1
    metadata:
    name: sample-dpa
    namespace: openshift-adp
    annotations:
        oadp.openshift.io/unsupported-velero-server-args: '{"arg_1":"val_1","arg2":"val2"}'
        oadp.openshift.io/unsupported-node-agent-args: '{"arg_1":"val_1","arg_2":"val_2"}'
    spec:
        [...]
    ```

 - Labels to include unsupported args directly

   This approach is similar to annotations, with all the pros/cons of them with additional cons of labels being designed for resource identification, querying, and organizing within clusters and not configurations.

## Security Considerations
Passing input data without validation may cause malformed configurations to be included.

This design does not allow to store sensitive information within configuration, as it's using ConfigMaps and not Secrets.

Proper RBAC access restriction to the relevant ConfigMaps is required to ensure least privilege is granted.

## Compatibility
This design is backwards compatible with previous unsupported arguments passed via DPA specification, however it takes precendence over them.

This may lead to the situation where users will have running previous configurations that are not reflected with the OADP instances that are using new way of defining unsupported arguments.

This will be documented in the developer and support documentation.

## Implementation
The implementation will take place within OADP controller only, without changes to the DPA specification.

Once ConfigMap(s) are discovered to be in the same Namespace as OADP their config will take precendence and replace all the other arguments passed to the executable as a simple string without any validations.

There may be slight modification to the argument name or it's value taken from the `ConfigMap` to align with the `Cobra` library as described in the [notes](#notes).
