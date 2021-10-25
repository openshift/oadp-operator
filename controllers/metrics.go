package controllers

import (
	"context"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/log"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var (
	// 'backupPhase' - [New, FailedValidation, InProgress, Uploading, UploadingPartialFailure, Completed, PartiallyFailed, Failed, Deleting]
	// 'restorePhase' - [New, FailedValidation, InProgress, Completed, PartiallyFailed, Failed]
	backupGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oadp_app_workload_backup_total",
		Help: "Count of OADP backups sorted by phase",
	},
		[]string{"phase"},
	)

	restoreGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oadp_app_workload_restore_total",
		Help: "Count of OADP restores sorted by phase",
	},
		[]string{"phase"},
	)

	oadpGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oadp_app_instances",
		Help: "Count of OADP instances sorted by status",
	},
		[]string{"status"},
	)
)

func recordOADPMetrics(client client.Client) {
	const (
		// OADP CR status
		Upgradeable = "Upgradeable"
		Progressing = "Progressing"
		Available   = "Available"
		Degraded    = "Degraded"
	)

	go func() {
		time.Sleep(10 * time.Second)

		for {
			// get all OADP Velero CRs
			veleroCRs := oadpv1alpha1.VeleroList{}
			err := client.List(context.TODO(), &veleroCRs)

			// retry if errored
			if err != nil {
				log.Info("Metrics veleroCRs list error: " + err.Error())
				continue
			}

			// Holding counters to make gauge update atomic
			var oadpUpgradeable, oadpProgressing, oadpAvailable, oadpDegraded float64

			// for all OADP Velero CRs, count # in each phase
			// TODO: Remove Conditions[0] and loop through the list and check for the type and true status
			for _, v := range veleroCRs.Items {
				if v.Status.Conditions[0].Type == Upgradeable && v.Status.Conditions[0].Status == "True" {
					oadpUpgradeable++
					continue
				}

				if v.Status.Conditions[0].Type == Progressing && v.Status.Conditions[0].Status == "True" {
					oadpProgressing++
					continue
				}

				if v.Status.Conditions[0].Type == Available && v.Status.Conditions[0].Status == "True" {
					oadpAvailable++
					continue
				}

				if v.Status.Conditions[0].Type == Degraded && v.Status.Conditions[0].Status == "True" {
					oadpDegraded++
					continue
				}
			}

			oadpGauge.With(
				prometheus.Labels{"status": Upgradeable}).Set(oadpUpgradeable)
			oadpGauge.With(
				prometheus.Labels{"status": Progressing}).Set(oadpProgressing)
			oadpGauge.With(
				prometheus.Labels{"status": Available}).Set(oadpAvailable)
			oadpGauge.With(
				prometheus.Labels{"status": Degraded}).Set(oadpDegraded)

		}

	}()
}

func recordBackupMetrics(client client.Client) {

	const (
		// backup Phases
		New                     = "New"
		FailedValidation        = "FailedValidation"
		InProgress              = "InProgress"
		Uploading               = "Uploading"
		UploadingPartialFailure = "UploadingPartialFailure"
		Completed               = "Completed"
		PartiallyFailed         = "PartiallyFailed"
		Failed                  = "Failed"
		Deleting                = "Deleting"
	)

	go func() {
		for {
			time.Sleep(10 * time.Second)

			// get all backup objects
			backups := velerov1.BackupList{}
			err := client.List(context.TODO(), &backups)

			// retry if errored
			if err != nil {
				log.Info("Metrics Backups list error: " + err.Error())
				continue
			}

			// Holding counters to make gauge update atomic
			var backupNew, backupFailedValidation, backupInProgress, backupUploading, backupUploadingPartialFailure, backupCompleted, backupPartiallyFailed, backupFailed, backupDeleting float64

			// for all backups, count # in each phase
			for _, b := range backups.Items {
				if b.Status.Phase == New {
					backupNew++
					continue
				}
				if b.Status.Phase == FailedValidation {
					backupFailedValidation++
					continue
				}
				if b.Status.Phase == InProgress {
					backupInProgress++
					continue
				}
				if b.Status.Phase == Uploading {
					backupUploading++
					continue
				}
				if b.Status.Phase == UploadingPartialFailure {
					backupUploadingPartialFailure++
					continue
				}
				if b.Status.Phase == Completed {
					backupCompleted++
					continue
				}
				if b.Status.Phase == PartiallyFailed {
					backupPartiallyFailed++
					continue
				}
				if b.Status.Phase == Failed {
					backupFailed++
					continue
				}
				if b.Status.Phase == Deleting {
					backupDeleting++
					continue
				}
			}

			backupGauge.With(
				prometheus.Labels{"phase": New}).Set(backupNew)
			backupGauge.With(
				prometheus.Labels{"phase": FailedValidation}).Set(backupFailedValidation)
			backupGauge.With(
				prometheus.Labels{"phase": InProgress}).Set(backupInProgress)
			backupGauge.With(
				prometheus.Labels{"phase": Uploading}).Set(backupUploading)
			backupGauge.With(
				prometheus.Labels{"phase": UploadingPartialFailure}).Set(backupUploadingPartialFailure)
			backupGauge.With(
				prometheus.Labels{"phase": Completed}).Set(backupCompleted)
			backupGauge.With(
				prometheus.Labels{"phase": PartiallyFailed}).Set(backupPartiallyFailed)
			backupGauge.With(
				prometheus.Labels{"phase": Failed}).Set(backupFailed)
			backupGauge.With(
				prometheus.Labels{"phase": Deleting}).Set(backupDeleting)
		}
	}()
}

func recordRestoreMetrics(client client.Client) {

	const (
		// restore Phases
		New              = "New"
		FailedValidation = "FailedValidation"
		InProgress       = "InProgress"
		Completed        = "Completed"
		PartiallyFailed  = "PartiallyFailed"
		Failed           = "Failed"
	)

	go func() {
		for {
			time.Sleep(10 * time.Second)

			// get all restore objects
			restores := velerov1.RestoreList{}
			err := client.List(context.TODO(), &restores)

			// retry if errored
			if err != nil {
				log.Info("Metrics Restores list error: " + err.Error())
				continue
			}

			// Holding counters to make gauge update atomic
			var restoreNew, restoreFailedValidation, restoreInProgress, restoreCompleted, restorePartiallyFailed, restoreFailed float64

			// for all backups, count # in each phase
			for _, r := range restores.Items {
				if r.Status.Phase == New {
					restoreNew++
					continue
				}
				if r.Status.Phase == FailedValidation {
					restoreFailedValidation++
					continue
				}
				if r.Status.Phase == InProgress {
					restoreInProgress++
					continue
				}
				if r.Status.Phase == Completed {
					restoreCompleted++
					continue
				}
				if r.Status.Phase == PartiallyFailed {
					restorePartiallyFailed++
					continue
				}
				if r.Status.Phase == Failed {
					restoreFailed++
					continue
				}
			}

			restoreGauge.With(
				prometheus.Labels{"phase": New}).Set(restoreNew)
			restoreGauge.With(
				prometheus.Labels{"phase": FailedValidation}).Set(restoreFailedValidation)
			restoreGauge.With(
				prometheus.Labels{"phase": InProgress}).Set(restoreInProgress)
			restoreGauge.With(
				prometheus.Labels{"phase": Completed}).Set(restoreCompleted)
			restoreGauge.With(
				prometheus.Labels{"phase": PartiallyFailed}).Set(restorePartiallyFailed)
			restoreGauge.With(
				prometheus.Labels{"phase": Failed}).Set(restoreFailed)
		}
	}()
}
