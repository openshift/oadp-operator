# Kubectl-oadp plugin design

## Abstract
The purpose of the kubectl-oadp plugin is to allow the customer to create and delete backups, along with creating restores in OADP without needing to alias velero to do so. Non-cluster admins should also be able to create NABs (Non-Admin Backups) and get the logs from them.

## Background
The current OpenShift cli is suboptimal as oc backup delete $foo deletes the k8s object instead of the backup but velero backup delete $foo deletes the backup, along with the backup files in storage. Currently, customers would need to alias velero in order to delete their backups, which is not ideal. The purpose of kubectl-oadp would be to make the cli experience better and easier to use along with enabling users to be able to get the logs of the backups.

## Goals
- Customers can create, delete, and get the logs of the backups and restores
- A non-cluster admin can create, delete and receive the logs of the Non-Admin-Backups (NAB)

## Non-Goals
- Non-Admin-Restore and other Non-Admin CRs due to time constraints

## Use-Case
A use case of the kubectl-oadp cli could be when a non-cluster admin would like to create a NAB or view the logs of a NAB without having to depend on the cluster admins to do so. Another use case would be if a developer would want to create a normal backup, they can just use this plugin to do so.

## High-Level Design
Creating a kubectl plugin (kubectl-oadp) will be a good solution to the problem at hand. It will be able to create/delete backups and restores utilizing the go package imports. Non-cluster admin will be able to create NABs without the need for cluster admin to do it for them. A way to distinguish between creating either NABs or regular backups would be in the cli using the non-admin api. For instance, if you would like to create a NAB, you would have to do kubectl oadp nonadmin backup create [backupname].  

## Detailed Design
The kubectl plugin will have imports from velero to help with the creation/deletion of backups and restores. It will be written in Golang and be using cobra for command-line parsing. The non-admin cli can be a subset of some backup clis that already exist such as backup.go and create.go. 

What we discovered with the regular commands such as version, backup, and restore we can just import the libraries 

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli/restore"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli/version"
)
```
With non-admin, we would have to create the cli ourselves since there are no CLIs for it.

```go
func NewCreateCommand(f client.Factory, use string) *cobra.Command {
	o := NewCreateOptions()

	c := &cobra.Command{
		Use:   use + " NAME",
		Short: "Create a non-admin backup",
		Args:  cobra.MaximumNArgs(1),
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(o.Complete(args, f))
			cmd.CheckError(o.Validate(c, args, f))
			cmd.CheckError(o.Run(c, f))
		}, 

```
CLI Examples
```sh
kubectl oadp backup create
kubectl oadp backup delete 
kubectl oadp nonadmin backup create <BACKUP_NAME>
kubectl oadp nonadmin backup logs
kubectl oadp nonadmin backup describe 
kubectl oadp restore create
```

## Alternatives Considered
An alternative that was considered was creating our own CLI from scratch and not using a plugin. We can instead use the existing oc commands and just add on to them with a kubectl plugin. 

Aliasing is another way in which you could access the velero command line. However, this is not ideal because non admins do not have permission to use the velero cli, so the kubectl plugin would allow non admins to retrieve their non admin backups similar to admins via velero cli. 

## Security Considerations
The security for the plugin is controlled by OpenShift RBAC, enabling cluster admins to manage user permissions. This is utilized to restrict users to commands permitted within their namespace. The plugin also generates error messages like "Unauthorized Access" when users attempt commands without proper permissions.

## Compatibility
This plugin would need to be updated so that it would be importing the right version of the velero backup and restore libraries.

## Future Work
Some future work that could be expanded upon would be Non-admin Restores, and other Non-admin CRs such as NonAdminBackupStorageLocation. These would allow more options for those who would like to use different non admin commands.
