package e2e_test

import (
	"log"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
)

var _ = Describe("Backup and restore tests with must-gather", func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = AfterEach(func(ctx SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	DescribeTable("Backup and restore applications and run must-gather",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, expectedErr, updateLastBRcase, updateLastInstallTime)

			// TODO look for duplications in tearDownBackupAndRestore
			baseReportDir := artifact_dir + "/" + brCase.Name
			err := os.MkdirAll(baseReportDir, 0755)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Running must gather for backup/restore test - " + "")
			err = RunMustGather(oc_cli, baseReportDir+"/must-gather")
			if err != nil {
				log.Printf("Failed to run must gather: " + err.Error())
			}
			Expect(err).ToNot(HaveOccurred())
			// get dirs in must-gather dir
			dirEntries, err := os.ReadDir(baseReportDir + "/must-gather")
			Expect(err).ToNot(HaveOccurred())
			clusterDir := ""
			for _, dirEntry := range dirEntries {
				if dirEntry.IsDir() && strings.HasPrefix(dirEntry.Name(), "quay-io") {
					mustGatherImageDir := baseReportDir + "/must-gather/" + dirEntry.Name()
					// extract must-gather.tar.gz
					err = utils.ExtractTarGz(mustGatherImageDir, "must-gather.tar.gz")
					Expect(err).ToNot(HaveOccurred())
					mustGatherDir := mustGatherImageDir + "/must-gather"
					clusters, err := os.ReadDir(mustGatherDir + "/clusters")
					Expect(err).ToNot(HaveOccurred())
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
					Expect(err).ToNot(HaveOccurred())
				}
			}
			if brCase.MustGatherValidationFunction != nil && clusterDir != "" {
				err = (*brCase.MustGatherValidationFunction)(clusterDir)
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("Mongo application DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   mongoready(true, false, CSIDataMover),
				PostRestoreVerify: mongoready(false, false, CSIDataMover),
				BackupTimeout:     20 * time.Minute,
			},
			MustGatherFiles: []string{
				"namespaces/" + namespace + "/oadp.openshift.io/dpa-ts-" + instanceName + "/ts-" + instanceName + ".yml",
				"namespaces/" + namespace + "/velero.io/backupstoragelocations.velero.io/ts-" + instanceName + "-1.yaml",
			},
		}, nil),
	)
})
