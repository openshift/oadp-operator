Kubectl-oadp plugin design 

Abstract
The purpose of this plugin is to allow the customer to delete backups, along with creating restores in OADP without needing to alias velero to do so. 

Background
The current OADP cli is suboptimal as oc backup delete $foo deletes the k8 object instead of the backup but velero backup delete $foo deletes the backup, along with the backup files in storage. Currently, customers would need to alias velero in order to delete their backups, which is not ideal. The purpose of kubectl-oadp would be to make the cli experience better and easier to use along with enabling users to be able to get the logs of the backups. 

Goals
- Customers can create backups and restores
- A non-cluster admin can create Non-Admin-Backups (NAB)

High-Level Design
Creating a kubectl plugin (kubctl-oadp) will be a good solution to the problem at hand. It will be able to create/delete backups and restores. Non-cluster admin will be able to create NABs without the need for cluster admin to do it for them. 

Detailed Design
The kubectl plugin will have imports from velero to help with the creation/deletion of backups and restores. It will be written in Golang and be using cobra for command-line parsing. The non-admin cli can be a subset of some backup clis that already exist such as backup.go and create.go. 

CLI Examples

oc oadp backup create 
oc oadp backup logs
oc oadp restore create
oc oadp restore logs 

