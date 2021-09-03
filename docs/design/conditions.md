# Status Condition Types and Usage

For OADP operator, we will be updating the conditions on the status subresource
to represent the state of the operand. There are 4 types of conditions our
operator can set:
1. `Upgradeable`
2. `Progressing`
3. `Available`
4. `Degraded`

## Upgradeable

The `Upgradeable` condition is used to tell OLM whether or not our operator is
safe to upgrade. The default state of this condition should be `true`. If
Velero is currently running a backup or a restore, then we will want to set
`Upgradeable` to `false` to ensure the velero deployment is not rolled out
while a backup or a restore is running. Some examples:
- A velero `Backup` is `InProgress`
- A velero `Restore` is `InProgress`

## Progressing

The `Progressing` condition is set to `true` when resources our operator
manages are created, updated, or scaling up/down. Some examples:
- The velero deployment is created
- The velero deployment is rolling out
- The restic daemonset is created
- The restic daemonset is scaling up or down

## Available

The `Available` condition is set to `true` when all of the resources our
operator manages have been updated to the latest version, and all requested
updates have been completed. Some examples:
- The velero deployment replica is available
- No old velero deployment replicas are running
- The restic daemonset replicas are available
- No old restic daemonset replicas are running

## Degraded

The `Degraded` condition is set to `true` when any of the
deployments/daemonsets our operator manages are in an errored state. This
condition can also be used to indicate that there are missing resources in the
cluster preventing our operand from providing its expected functionality. Some
examples:
- The deployments/daemonsets are in a `CrashLoopBackoff` state
- The deployments/daemonsets are reporting `ImagePullBackoff` errors
- Readiness probe failures
- Insufficient quota


# Current plan

We will update `ReconcileBatch` to support returning information about whether
the operator is `Progressing` or not. Then if we detect an error, we will set
the `Degraded` condition to `true`. If there is no error we will set
`Available` condition to `true If there is no error we will set `Available`
condition to `true`.

For the `Upgradeable` condition, we will not manage it from `ReconcileBatch`.
Since the only use case we currently have for `Upgradeable` comes from running
backup/restores, we will introduce 2 new controllers which will watch `Backups`
and `Restores` respectively and set the `Upgradeable` condition based on if any
of those are `InProgress`.

** NOTE ** This document assumes the registry deployment has been removed. If
it is not removed, then the registry deployment/resources need to be included
as well
