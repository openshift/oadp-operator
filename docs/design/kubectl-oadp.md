# Kubectl-oadp plugin design

## Abstract
The purpose of this plugin is to allow the customer to create and delete backups, along with creating restores in OADP without needing to alias velero to do so.

## Background
The current OADP cli is suboptimal as oc backup delete $foo deletes the k8 object instead of the backup but velero backup delete $foo deletes the backup, along with the backup files in storage. Currently, customers would need to alias velero in order to delete their backups, which is not ideal. The purpose of kubectl-oadp would be to make the cli experience better and easier to use along with enabling users to be able to get the logs of the backups.

## Goals
- Customers can create and delete backups
- A non-cluster admin can create Non-Admin-Backups (NAB)

## Non-Goals
- Non-Admin-Restore due to time constraints

## High-Level Design
Creating a kubectl plugin (kubectl-oadp) will be a good solution to the problem at hand. It will be able to create/delete backups and restores. Non-cluster admin will be able to create NABs without the need for cluster admin to do it for them. 

## Detailed Design
The kubectl plugin will have imports from velero to help with the creation/deletion of backups and restores. It will be written in Golang and be using cobra for command-line parsing. The non-admin cli can be a subset of some backup clis that already exist such as backup.go and create.go. 

What we discovered with the regular commands such as version, backup, and restore we can just import the libraries 

```sh
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
With non-admin, we would have to create the cli ourselves since there are no cli’s for it.

```sh
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
oc oadp backup create 
oc oadp backup logs
oc oadp restore create
oc oadp restore logs 
```

## Alternatives Considered
An alternative that was considered was creating our own CLI from scratch and not using a plugin. We can instead use the existing oc commands and just add on to them with a kubectl plugin. 

Aliasing is another way in which you could access the velero command line. However, this is not ideal because some individuals do not have permission to use the velero cli, so the kubectl plugin would allow those people to use velero cli. 

## Security Considerations
The user first enters in a command in which the plugin reads the command and creates a velero client factory. The client factory then creates a connection to the kubernetes server.

## Compatibility
This plugin would need to be updated so that it would be importing the right version of the velero backup and restore libraries. 
