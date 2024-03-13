# notes:
Obviously there are improvements to this workflow, however this is meant to
serve as a template or example to fit into other working test frameworks.

# To install
setup the openshift-virtualization operator

# Start vm
`oc create -f namemspace-virttodo.yaml -f virtualmachine-fedora-close-carp.yaml -f service.yaml -f vmroute.yaml`

# Check volumeMode
`oc get pvc fedora-close-carp -o yaml -n virttodo | grep volumeMode`

# Wait for the cloud-init scripts to:
* install mariadb
* install and start the todolist app

* login w/ test/dog8code
`tail -f /var/log/cloud-init-output.log`

# Add data to the todolist
* get the route
* `oc get route -n virttodo`
* open browser to $route
  * add data

# Build out the test.py
* edit the test.py script, add the route prepended with http://
`python test.py`

# Backup vm
`oc create -f backup_virttodo.yaml`

# Delete everything
`oc delete -f namemspace-virttodo.yaml -f virtualmachine-fedora-close-carp.yaml -f service.yaml -f vmroute.yaml`

# Restore vm
`oc create -f restore_virttodo.yaml`

# Check the data
* compare the data you entered prior to the backup with the current data
