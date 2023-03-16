# Create a project and user with non-admin access that can execute an OADP backup

## steps

Using an example with a user called test05 in a project call test05
```
./install.sh -p test05 -u test05 -d /tmp/test05
```

The user and project will be created, however the user needs a password.

* follow the documented steps here: https://www.redhat.com/sysadmin/openshift-htpasswd-oauth

```
htpasswd  -B -b ~/OADP/passwd_file test05 passw0rd
```

* Typically I destroy the old htpasswd config in OCP and recreate
* Upload the file or paste creds


## In another browser
* log in as the new user and kick off the pipeline 
