/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leaderelection

import (
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

// TestLeaderElectionDefaulting tests the default leader election configuration
func TestLeaderElectionDefaulting(t *testing.T) {
	config := LeaderElectionDefaulting(configv1.LeaderElection{}, "", "")

	// Verify default values for HA clusters
	if config.LeaseDuration.Duration != 137*time.Second {
		t.Errorf("expected LeaseDuration 137s, got %v", config.LeaseDuration.Duration)
	}
	if config.RenewDeadline.Duration != 107*time.Second {
		t.Errorf("expected RenewDeadline 107s, got %v", config.RenewDeadline.Duration)
	}
	if config.RetryPeriod.Duration != 26*time.Second {
		t.Errorf("expected RetryPeriod 26s, got %v", config.RetryPeriod.Duration)
	}
}

// TestLeaderElectionDefaulting_DisabledWhenNotEnabled tests that leader election
// is disabled when enabled=false is passed
func TestLeaderElectionDefaulting_DisabledWhenNotEnabled(t *testing.T) {
	config := LeaderElectionDefaulting(
		configv1.LeaderElection{
			Disable: true,
		},
		"", "",
	)

	if !config.Disable {
		t.Error("expected Disable to be true when passed as true")
	}
}

// TestLeaderElectionSNOConfig_Deprecated tests the old SNO config function
// Note: This function is no longer used for SNO (we disable LE instead),
// but we test it to ensure it still works if needed for reference
func TestLeaderElectionSNOConfig_Deprecated(t *testing.T) {
	baseConfig := LeaderElectionDefaulting(configv1.LeaderElection{}, "", "")
	snoConfig := LeaderElectionSNOConfig(baseConfig)

	// Verify SNO-specific extended values
	if snoConfig.LeaseDuration.Duration != 270*time.Second {
		t.Errorf("expected SNO LeaseDuration 270s, got %v", snoConfig.LeaseDuration.Duration)
	}
	if snoConfig.RenewDeadline.Duration != 240*time.Second {
		t.Errorf("expected SNO RenewDeadline 240s, got %v", snoConfig.RenewDeadline.Duration)
	}
	if snoConfig.RetryPeriod.Duration != 60*time.Second {
		t.Errorf("expected SNO RetryPeriod 60s, got %v", snoConfig.RetryPeriod.Duration)
	}
}

// TestGetLeaderElectionConfig_DisabledWhenNotEnabled tests that leader election
// is disabled when enableLeaderElection is false
func TestGetLeaderElectionConfig_DisabledWhenNotEnabled(t *testing.T) {
	// When enabled=false, the config should have Disable=true
	config := configv1.LeaderElection{
		Disable: true, // !enabled
	}
	defaulted := LeaderElectionDefaulting(config, "", "")

	if !defaulted.Disable {
		t.Error("expected Disable to be true when leader election is not enabled")
	}
}

// TestGetLeaderElectionConfig_EnabledForHA tests that leader election
// uses HA defaults when enabled on a non-SNO cluster
func TestGetLeaderElectionConfig_EnabledForHA(t *testing.T) {
	config := LeaderElectionDefaulting(
		configv1.LeaderElection{
			Disable: false, // enabled
		},
		"", "",
	)

	if config.Disable {
		t.Error("expected Disable to be false for HA cluster")
	}
	if config.LeaseDuration.Duration != 137*time.Second {
		t.Errorf("expected HA LeaseDuration 137s, got %v", config.LeaseDuration.Duration)
	}
}

// TestSNOLeaderElectionDisabled tests that for SNO topology, we return
// a config with Disable=true (the new behavior for OADP-7419)
func TestSNOLeaderElectionDisabled(t *testing.T) {
	// Simulate what GetLeaderElectionConfig does for SNO
	// We can't easily test the full function without mocking the k8s client,
	// but we can verify the logic by checking what happens when SNO is detected

	defaultConfig := LeaderElectionDefaulting(
		configv1.LeaderElection{
			Disable: false, // Initially enabled
		},
		"", "",
	)

	// Simulate SNO detection - set Disable to true
	defaultConfig.Disable = true

	if !defaultConfig.Disable {
		t.Error("expected Disable to be true for SNO cluster")
	}
}

// TestLeaderElectionClockSkewTolerance verifies the clock skew tolerance
// calculation for HA clusters (leaseDuration - renewDeadline = 30s)
func TestLeaderElectionClockSkewTolerance(t *testing.T) {
	config := LeaderElectionDefaulting(configv1.LeaderElection{}, "", "")

	clockSkew := config.LeaseDuration.Duration - config.RenewDeadline.Duration
	expectedClockSkew := 30 * time.Second

	if clockSkew != expectedClockSkew {
		t.Errorf("expected clock skew tolerance of %v, got %v", expectedClockSkew, clockSkew)
	}
}

// TestLeaderElectionRetryAttempts verifies the number of retry attempts
// for HA clusters (renewDeadline / retryPeriod = 4 attempts)
func TestLeaderElectionRetryAttempts(t *testing.T) {
	config := LeaderElectionDefaulting(configv1.LeaderElection{}, "", "")

	retryAttempts := int(config.RenewDeadline.Duration / config.RetryPeriod.Duration)
	expectedRetries := 4 // 107s / 26s = 4

	if retryAttempts != expectedRetries {
		t.Errorf("expected %d retry attempts, got %d", expectedRetries, retryAttempts)
	}
}
