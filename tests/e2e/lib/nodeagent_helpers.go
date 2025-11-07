package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/oadp-operator/pkg/common"
)

func GetNodeAgentDaemonSet(c *kubernetes.Clientset, namespace string) (*appsv1.DaemonSet, error) {
	nodeAgent, err := c.AppsV1().DaemonSets(namespace).Get(context.Background(), common.NodeAgent, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return nodeAgent, nil
}

func AreNodeAgentPodsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking for correct number of running Node Agent Pods...")
	return func() (bool, error) {
		nodeAgentDaemonSet, err := GetNodeAgentDaemonSet(c, namespace)
		if err != nil {
			return false, err
		}

		numScheduled := nodeAgentDaemonSet.Status.CurrentNumberScheduled
		numDesired := nodeAgentDaemonSet.Status.DesiredNumberScheduled
		// check correct number of NodeAgent Pods are initialized
		if numScheduled != numDesired {
			return false, fmt.Errorf("wrong number of Node Agent Pods")
		}
		if numDesired == 0 {
			return true, nil
		}

		podList, err := GetAllPodsWithLabel(c, namespace, "name="+common.NodeAgent)
		if err != nil {
			return false, err
		}

		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return false, fmt.Errorf("not all Node Agent Pods are running")
			}
		}
		return true, nil
	}
}

// keep for now
func IsNodeAgentDaemonSetDeleted(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking if NodeAgent DaemonSet has been deleted...")
	return func() (bool, error) {
		_, err := GetNodeAgentDaemonSet(c, namespace)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func NodeAgentDaemonSetHasNodeSelector(c *kubernetes.Clientset, namespace, key, value string) wait.ConditionFunc {
	return func() (bool, error) {
		ds, err := GetNodeAgentDaemonSet(c, namespace)
		if err != nil {
			return false, err
		}
		// verify DaemonSet has nodeSelector "foo": "bar"
		return ds.Spec.Template.Spec.NodeSelector[key] == value, nil
	}
}

// PrintNodeAgentDiagnostics prints node-agent pod logs filtered for PrepareRepo-related messages
// to help diagnose PrepareRepo bugs where local Kopia config files are missing
func PrintNodeAgentDiagnostics(c *kubernetes.Clientset, namespace string) {
	log.Println("===== Node-Agent Pod Diagnostics (PrepareRepo Bug Detection) =====")

	// Get all node-agent pods
	podList, err := GetAllPodsWithLabel(c, namespace, "name="+common.NodeAgent)
	if err != nil {
		log.Printf("ERROR: Failed to get node-agent pods: %v", err)
		return
	}

	if len(podList.Items) == 0 {
		log.Println("INFO: No node-agent pods found")
		return
	}

	for _, pod := range podList.Items {
		log.Printf("\n--- Node-Agent Pod: %s (Node: %s) ---", pod.Name, pod.Spec.NodeName)
		log.Printf("  Phase: %s", pod.Status.Phase)
		log.Printf("  Created: %s", pod.CreationTimestamp)

		// Get pod logs
		req := c.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			TailLines: int64Ptr(500), // Last 500 lines
		})
		podLogs, err := req.Stream(context.Background())
		if err != nil {
			log.Printf("  ERROR: Failed to get logs: %v", err)
			continue
		}
		defer podLogs.Close()

		// Read and filter logs
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(podLogs)
		if err != nil {
			log.Printf("  ERROR: Failed to read logs: %v", err)
			continue
		}

		logsStr := buf.String()
		lines := splitLines(logsStr)

		// Look for PrepareRepo-related log lines
		foundRepoInit := false
		foundConnect := false
		foundConfigError := false

		log.Println("  Relevant log entries:")
		for _, line := range lines {
			// Check for PrepareRepo evidence
			if containsAny(line, []string{"Repo has already been initialized", "repository already exists"}) {
				log.Printf("    ⚠️  %s", line)
				foundRepoInit = true
			}
			if containsAny(line, []string{"connecting to it", "Connect()"}) {
				log.Printf("    ✓ %s", line)
				foundConnect = true
			}
			if containsAny(line, []string{"config", "no such file", "ENOENT"}) {
				log.Printf("    🔴 %s", line)
				foundConfigError = true
			}
			// Also show DataUpload-related errors
			if containsAny(line, []string{"error", "failed", "Error"}) &&
				containsAny(line, []string{"DataUpload", "backup", "volume"}) {
				log.Printf("    ! %s", line)
			}
		}

		// Analyze findings
		if foundRepoInit && !foundConnect && foundConfigError {
			log.Println("  🔴 CRITICAL BUG EVIDENCE: Repository exists but Connect() was NOT called!")
			log.Println("  🔴 This confirms the PrepareRepo bug hypothesis")
		} else if foundRepoInit && !foundConnect {
			log.Println("  ⚠️  WARNING: Repository exists but no Connect() log found")
			log.Println("  ⚠️  This suggests PrepareRepo bug may have occurred")
		}
	}
	log.Println("================================================================")
}

// Helper functions for PrintNodeAgentDiagnostics

func int64Ptr(i int64) *int64 {
	return &i
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if containsSubstring(s, substr) {
			return true
		}
	}
	return false
}

func containsSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
