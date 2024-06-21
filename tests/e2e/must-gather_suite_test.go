package e2e_test

import (
	"log"
	"os"
	"strings"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var _ = ginkgov2.Describe("Backup and restore tests with must-gather", func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgov2.AfterEach(func(ctx ginkgov2.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgov2.DescribeTable("Backup and restore applications and run must-gather",
		func(brCase ApplicationBackupRestoreCase, expectedBackupErr, expectedRestoreErr error) {
			if ginkgov2.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgov2.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, expectedBackupErr, expectedRestoreErr, updateLastBRcase, updateLastInstallTime)

			// TODO look for duplications in tearDownBackupAndRestore
			baseReportDir := artifact_dir + "/" + brCase.Name
			err := os.MkdirAll(baseReportDir, 0755)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Running must gather for backup/restore test - " + "")
			err = lib.RunMustGather(oc_cli, baseReportDir+"/must-gather")
			if err != nil {
				log.Printf("Failed to run must gather: " + err.Error())
			}
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// get dirs in must-gather dir
			dirEntries, err := os.ReadDir(baseReportDir + "/must-gather")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			clusterDir := ""
			for _, dirEntry := range dirEntries {
				if dirEntry.IsDir() && strings.HasPrefix(dirEntry.Name(), "quay-io") {
					mustGatherImageDir := baseReportDir + "/must-gather/" + dirEntry.Name()
					// extract must-gather.tar.gz
					err = lib.ExtractTarGz(mustGatherImageDir, "must-gather.tar.gz")
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					mustGatherDir := mustGatherImageDir + "/must-gather"
					clusters, err := os.ReadDir(mustGatherDir + "/clusters")
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					for _, cluster := range clusters {
						if cluster.IsDir() {
							clusterDir = mustGatherDir + "/clusters/" + cluster.Name()
						}
					}
				}
			}
			if len(brCase.MustGatherFiles) > 0 && clusterDir != "" {
				for _, file := range brCase.MustGatherFiles {
					_, err := os.Stat(clusterDir + "/" + file)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
				}
			}
			if brCase.MustGatherValidationFunction != nil && clusterDir != "" {
				err = (*brCase.MustGatherValidationFunction)(clusterDir)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}
		},
		ginkgov2.Entry("Mongo application DATAMOVER", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
				BackupTimeout:     20 * time.Minute,
			},
			MustGatherFiles: []string{
				"namespaces/" + namespace + "/oadp.openshift.io/dpa-ts-" + instanceName + "/ts-" + instanceName + ".yml",
				"namespaces/" + namespace + "/velero.io/backupstoragelocations.velero.io/ts-" + instanceName + "-1.yaml",
			},
		}, nil, nil),
	)
})
