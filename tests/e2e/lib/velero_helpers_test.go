package lib

import (
	"reflect"
	"testing"
)

func Test_errorLogsExcludingIgnored(t *testing.T) {
	type args struct {
		logs string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			// sample from https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/openshift_oadp-operator/1126/pull-ci-openshift-oadp-operator-master-4.10-operator-e2e-aws/1690109468546699264#1:build-log.txt%3A686
			name: "error patch for managed fields are ignored",
			args: args{
				logs: `time="2023-08-11T22:02:39Z" level=debug msg="status field for endpointslices.discovery.k8s.io: exists: false, should restore: false" logSource="pkg/restore/restore.go:1487" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=error msg="error patch for managed fields mysql-persistent/mysql-6ztv6: endpointslices.discovery.k8s.io \"mysql-6ztv6\" not found" logSource="pkg/restore/restore.go:1516" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=info msg="Restored 40 items out of an estimated total of 50 (estimate will change throughout the restore)" logSource="pkg/restore/restore.go:669" name=mysql-6ztv6 namespace=mysql-persistent progress= resource=endpointslices.discovery.k8s.io restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=info msg="restore status includes excludes: <nil>" logSource="pkg/restore/restore.go:1189" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=debug msg="Skipping action because it does not apply to this resource" logSource="pkg/plugin/framework/action_resolver.go:61" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d`,
			},
			want: []string{},
		},
		{
			// sample from https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/openshift_oadp-operator/1126/pull-ci-openshift-oadp-operator-master-4.10-operator-e2e-aws/1690109468546699264#1:build-log.txt%3A686
			name: "error NOT for patch managed fields are NOT ignored",
			args: args{
				logs: `time="2023-08-11T22:02:39Z" level=debug msg="status field for endpointslices.discovery.k8s.io: exists: false, should restore: false" logSource="pkg/restore/restore.go:1487" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=error msg="error patch NOT for managed fields mysql-persistent/mysql-6ztv6: endpointslices.discovery.k8s.io \"mysql-6ztv6\" not found" logSource="pkg/restore/restore.go:1516" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=info msg="Restored 40 items out of an estimated total of 50 (estimate will change throughout the restore)" logSource="pkg/restore/restore.go:669" name=mysql-6ztv6 namespace=mysql-persistent progress= resource=endpointslices.discovery.k8s.io restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=info msg="restore status includes excludes: <nil>" logSource="pkg/restore/restore.go:1189" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d
time="2023-08-11T22:02:39Z" level=debug msg="Skipping action because it does not apply to this resource" logSource="pkg/plugin/framework/action_resolver.go:61" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d`,
			},
			want: []string{`time="2023-08-11T22:02:39Z" level=error msg="error patch NOT for managed fields mysql-persistent/mysql-6ztv6: endpointslices.discovery.k8s.io \"mysql-6ztv6\" not found" logSource="pkg/restore/restore.go:1516" restore=openshift-adp/mysql-twovol-csi-e2e-76673cb9-3892-11ee-b9ab-0a580a83082d`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorLogsExcludingIgnored(tt.args.logs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("errorLogsExcludingIgnored() = %v, want %v", got, tt.want)
			}
		})
	}
}
