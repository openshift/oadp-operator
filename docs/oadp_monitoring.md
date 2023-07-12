<hr style="height:1px;border:none;color:#333;">
<h1 align="center">OADP Observability</h1>
<h2 align="center">Using OpenShift User Workload Monitoring for OADP</h2>

## Preface

The OpenShift Container Platform provides a [monitoring stack](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/monitoring/index) that allows users and administrators to effectively monitor and manage their OpenShift clusters, as well as monitor and analyze the workload performance of user applications and services running on the clusters including receiving alerts when some events occurs.

The OADP Operator leverages an OpenShift [User Workload Monitoring](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/monitoring/enabling-monitoring-for-user-defined-projects) provided by the OpenShift Monitoring Stack for retrieving number of [metrics](#metrics) from the Velero service endpoint. The monitoring stack allows creating user-defined Alerting Rules or querying metrics using the OpenShift Metrics query front-end.

With enabled User Workload Monitoring it is also possible to configure and use any Prometheus-compatible third-party UI, such as Grafana to visualize Velero metrics. Please note that the usage of third-party UIs falls outside the scope of this document.


## OADP Monitoring Setup

Monitoring [metrics](#metrics) requires enabling monitoring for the user-defined projects and creating ServiceMonitor resource to scrape those metrics from the already enabled OADP service endpoint that lives in the `openshift-adp` namespace.

> **Note:** The configuration files that allows to enable User Workload Monitoring can be found in the [docs/examples/manifests/user_monitoring/](./examples/manifests/user_monitoring/) folder of this repository, however they will override any previous configuration that could have been on the cluster, so use them with caution. They are named with the number prefix, which represents the order in which they should be applied.

### Prerequisites
* OADP operator, a credentials secret, and a DataProtectionApplication (DPA) CR are all created. Follow [these steps](/docs/install_olm.md) for installation instructions.
* shell with the `oc` CLI command available and admin level access to the OpenShift cluster.


### Enable and Configure User Workload Monitoring

This paragraph will provide a short set of instructions how to enable user workload monitoring for an OADP project in the cluster. For comprehensive set of configuration options refer to the [enabling monitoring for user-defined projects](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/monitoring/enabling-monitoring-for-user-defined-projects#doc-wrapper) documentation.


1. Edit the `cluster-monitoring-config` ConfigMap object in the `openshift-monitoring` namespace and add or enable the `enableUserWorkload` option under `data/config.yaml`. 

    ```shell
    $ oc edit configmap cluster-monitoring-config -n openshift-monitoring
    ```


    ```yaml
    apiVersion: v1
    data:
      config.yaml: |
        enableUserWorkload: true  # Add this option or set to true
    kind: ConfigMap
    metadata:
    # [...]
    ```

2. After a short period of time verify the User Workload Monitoring Setup by checking if the following components are up and running in the `openshift-user-workload-monitoring` namespace

    ```shell
    $ oc get pods -n openshift-user-workload-monitoring
    NAME                                   READY   STATUS    RESTARTS   AGE
    prometheus-operator-6844b4b99c-b57j9   2/2     Running   0          43s
    prometheus-user-workload-0             5/5     Running   0          32s
    prometheus-user-workload-1             5/5     Running   0          32s
    thanos-ruler-user-workload-0           3/3     Running   0          32s
    thanos-ruler-user-workload-1           3/3     Running   0          32s
    ```

3. Please verify the existence of the `user-workload-monitoring-config` ConfigMap in the `openshift-user-workload-monitoring`, and if it is found, skip the following two steps (4th and 5th) accordingly

    ```shell
    $ oc get configmap user-workload-monitoring-config -n openshift-user-workload-monitoring
    Error from server (NotFound): configmaps "user-workload-monitoring-config" not found

    # We need to create: user-workload-monitoring-config ConfigMap
    ```

4. Create `user-workload-monitoring-config` ConfigMap for the User Workload Monitoring and save it under `2_configure_user_workload_monitoring.yaml` filename

    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: user-workload-monitoring-config
      namespace: openshift-user-workload-monitoring
    data:
      config.yaml: |
    ```

5. Apply the `2_configure_user_workload_monitoring.yaml`

    ```shell
    $ oc apply -f 2_configure_user_workload_monitoring.yaml
    configmap/user-workload-monitoring-config created
    ```

### Create OADP Service Monitor

OADP provides an `openshift-adp-velero-metrics-svc` service which is being created when DPA is configured. The ServiceMonitor that is used by the user workload monitoring will need to point to that SVC service.

1. Ensure the `openshift-adp-velero-metrics-svc` exists. It should contain `app.kubernetes.io/name=velero` label which will be used as selector for our ServiceMonitor

    ```shell
    $ oc get svc -n openshift-adp -l app.kubernetes.io/name=velero
    NAME                               TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
    openshift-adp-velero-metrics-svc   ClusterIP   172.30.38.244   <none>        8085/TCP   1h
    ```

2. Create ServiceMonitor yaml file that matches our existing SVC label and save it under `3_create_oadp_service_monitor.yaml` name. That ServiceMonitor will be created in the `openshift-adp` namespace where our `openshift-adp-velero-metrics-svc` SVC lives

    ```yaml
    apiVersion: monitoring.coreos.com/v1
    kind: ServiceMonitor
    metadata:
      labels:
        app: oadp-service-monitor
      name: oadp-service-monitor
      namespace: openshift-adp
    spec:
      endpoints:
      - interval: 30s
        path: /metrics
        targetPort: 8085
        scheme: http
      selector:
        matchLabels:
          app.kubernetes.io/name: "velero"
    ```

3. Apply the `3_create_oadp_service_monitor.yaml`

    ```shell
    $ oc apply -f 3_create_oadp_service_monitor.yaml
    servicemonitor.monitoring.coreos.com/oadp-service-monitor created
    ```

4. Confirm that our new ServiceMonitor is `Up`

    In the **Administrator** perspective in the OpenShift Container Platform web console use **Observe/Targets** to view the Metrics Targets. Ensure the `Filter` is either unselected or `User`  Source is selected and type `openshift-adp` in the Text search field. Ensure the status for our ServiceMonitor is Up:
    
    ![OpenShift Metrics Targets](./images/metrics_targets.png)

## Sample Alerting Rules

 The OpenShift Container Platform monitoring stack allows to receive Alerts configured using certain Alerting Rules. To create Alerting rule for the OADP project we will use one of the [Metrics](#metrics), which are scraped with the user workload monitoring described previously.
 
Please refer to the OpenShift documentation for detailed instructions on how to create and manage [OpenShift Alerts](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/monitoring/managing-alerts).

1. Create PrometheusRule yaml file with our sample `OADPBackupFailing` alert and save it under `4_create_oadp_alert_rule.yaml` name.

    ```yaml
    apiVersion: monitoring.coreos.com/v1
    kind: PrometheusRule
    metadata:
      name: sample-oadp-alert
      namespace: openshift-adp
    spec:
      groups:
      - name: sample-oadp-backup-alert
        rules:
        - alert: OADPBackupFailing
          annotations:
            description: 'OADP had {{$value | humanize}} backup failures over the last 2 hours.'
            summary: OADP has issues creating backups
          expr: |
            increase(velero_backup_failure_total{job="openshift-adp-velero-metrics-svc"}[2h]) > 0
          for: 5m
          labels:
            severity: warning
    ```

    In the above example, we will see an Alert, when the increase of number of failing backups (which means there were new failures) over period of the 2 last hours was greater then 0 and this state persistet for at least 5 minutes. If the time of the first increase is lower then 5 minutes the Alert will be in a `Pending` sate after which it will turn into `Firing` state.

2. Apply the `4_create_oadp_alert_rule.yaml`, this will create our PrometheusRule in the `openshift-adp` namespace

    ```shell
    $ oc apply -f 4_create_oadp_alert_rule.yaml
    prometheusrule.monitoring.coreos.com/sample-oadp-alert created
    ```

3. Viewing the alerts

    When the Alert is triggered it can be viewed:
      - In the **Administrator** perspective view under **Observe/Alerting** menu. Ensure to select `User` in the Filter box, otherwise by default only `Platform` alerts are presented.

      - In the **Developer** view under **Observe** menu.

    ![OADP Failing Backup Alert](./images/oadp_backup_failing_alert.png)

## Metrics

### List of available metrics

Following is the list of metrics provided by the OADP together with their [Types](https://prometheus.io/docs/concepts/metric_types/)

| Metric Name | Description | Type |
| ----------- | ----------- | --- |
| kopia_content_cache_hit_bytes | Number of bytes retrieved from the cache | Counter |
| kopia_content_cache_hit_count | Number of time content was retrieved from the cache | Counter |
| kopia_content_cache_malformed | Number of times malformed content was read from the cache | Counter |
| kopia_content_cache_miss_count | Number of time content was not found in the cache and fetched | Counter |
| kopia_content_cache_missed_bytes | Number of bytes retrieved from the underlying storage | Counter |
| kopia_content_cache_miss_error_count | Number of time content could not be found in the underlying storage | Counter |
| kopia_content_cache_store_error_count | Number of time content could not be saved in the cache | Counter |
| kopia_content_get_bytes | Number of bytes retrieved using GetContent | Counter |
| kopia_content_get_count | Number of time GetContent() was called | Counter |
| kopia_content_get_error_count | Number of time GetContent() was called and the result was an error | Counter |
| kopia_content_get_not_found_count | Number of time GetContent() was called and the result was not found | Counter |
| kopia_content_write_bytes | Number of bytes passed to WriteContent() | Counter |
| kopia_content_write_count | Number of time WriteContent() was called | Counter |
| velero_backup_attempt_total | Total number of attempted backups | Counter |
| velero_backup_deletion_attempt_total | Total number of attempted backup deletions | Counter |
| velero_backup_deletion_failure_total | Total number of failed backup deletions | Counter |
| velero_backup_deletion_success_total | Total number of successful backup deletions | Counter |
| velero_backup_duration_seconds | Time taken to complete backup, in seconds | Histogram |
| velero_backup_failure_total | Total number of failed backups | Counter |
| velero_backup_items_errors | Total number of errors encountered during backup | Gauge |
| velero_backup_items_total | Total number of items backed up | Gauge |
| velero_backup_last_status | Last status of the backup. A value of 1 is success, 0 | Gauge |
| velero_backup_last_successful_timestamp | Last time a backup ran successfully, Unix timestamp in seconds | Gauge |
| velero_backup_partial_failure_total | Total number of partially failed backups | Counter |
| velero_backup_success_total | Total number of successful backups | Counter |
| velero_backup_tarball_size_bytes | Size, in bytes, of a backup | Gauge |
| velero_backup_total | Current number of existent backups | Gauge |
| velero_backup_validation_failure_total | Total number of validation failed backups | Counter |
| velero_backup_warning_total | Total number of warned backups | Counter |
| velero_csi_snapshot_attempt_total | Total number of CSI attempted volume snapshots | Counter |
| velero_csi_snapshot_failure_total | Total number of CSI failed volume snapshots | Counter |
| velero_csi_snapshot_success_total | Total number of CSI successful volume snapshots | Counter |
| velero_restore_attempt_total | Total number of attempted restores | Counter |
| velero_restore_failed_total | Total number of failed restores | Counter |
| velero_restore_partial_failure_total | Total number of partially failed restores | Counter |
| velero_restore_success_total | Total number of successful restores | Counter |
| velero_restore_total | Current number of existent restores | Gauge |
| velero_restore_validation_failed_total | Total number of failed restores failing validations | Counter |
| velero_volume_snapshot_attempt_total | Total number of attempted volume snapshots | Counter |
| velero_volume_snapshot_failure_total | Total number of failed volume snapshots | Counter |
| velero_volume_snapshot_success_total | Total number of successful volume snapshots | Counter |

### Viewing metrics using OpenShift Observe UI

All the metrics described in the [metrics](#metrics) can be viewed in the **Administrator** OpenShift view or the **Developer** view, that needs to have access to the `openshift-adp` project.

Refer to the more detailed instructions on the [Querying metrics](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/monitoring/querying-metrics) that are using [PromQL query language](https://prometheus.io/docs/prometheus/latest/querying/basics/).

Select the **Observe** menu and Choose **Metrics**, then:

 - In the **Developer** view use the **Custom query** or click on the **Show PromQL** link, type the query and press Enter.
 - In the **Administrator** view type the Expression in the text field and hit the **Run Queries** button.

![OADP Metrics Sample Query](./images/oadp_metrics_query.png)
