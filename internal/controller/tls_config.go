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

package controller

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// buildTLSConfig creates a TLS configuration based on the DPT spec and BSL spec.
// Priority order:
// 1. If skipTLSVerify is true → InsecureSkipVerify: true
// 2. If BSL has caCert → Use custom CA cert with system certs
// 3. Otherwise → Use system certs (default)
func buildTLSConfig(dpt *oadpv1alpha1.DataProtectionTest, bsl *velerov1.BackupStorageLocationSpec, logger logr.Logger, caCertData []byte) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	// Priority 3: Load default system CA certificates.
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to load system certificate pool %v", err)
	}

	// Priority 1: Check if skipTLSVerify is set
	if dpt.Spec.SkipTLSVerify {
		logger.Info("TLS verification disabled via skipTLSVerify")
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}

	// Priority 2: Check for custom CA cert in retrieved by the controller from BSL or SecretKeySelector and append.
	// caCertData should be passed as parameter to handle the BSL CAData and CACertRef fields
	if len(caCertData) > 0 {
		logger.Info("Custom CA certificate found in param")

		if !caCertPool.AppendCertsFromPEM(caCertData) {
			return nil, fmt.Errorf("failed to parse CA certificates from param")
		}

		logger.Info("Successfully configured custom CA certificate from param")
	} else if bsl != nil && bsl.ObjectStorage != nil && len(bsl.ObjectStorage.CACert) > 0 {
		logger.Info("Custom CA certificate found in BSL")
		if !caCertPool.AppendCertsFromPEM(bsl.ObjectStorage.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificates from BSL")
		}
		logger.Info("Successfully configured custom CA certificate from BSL")
	}

	tlsConfig.RootCAs = caCertPool
	return tlsConfig, nil
}

// buildHTTPClientWithTLS creates an HTTP client with the appropriate TLS configuration
func buildHTTPClientWithTLS(dpt *oadpv1alpha1.DataProtectionTest, bsl *velerov1.BackupStorageLocationSpec, logger logr.Logger, caCertData []byte) (*http.Client, error) {
	tlsConfig, err := buildTLSConfig(dpt, bsl, logger, caCertData)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client, nil
}

// buildAWSSessionWithTLS creates an AWS session with the appropriate TLS configuration
func buildAWSSessionWithTLS(dpt *oadpv1alpha1.DataProtectionTest, bsl *velerov1.BackupStorageLocationSpec, region, endpoint string, logger logr.Logger, caCertData []byte) (*session.Session, error) {
	tlsConfig, err := buildTLSConfig(dpt, bsl, logger, caCertData)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	awsConfig := &aws.Config{
		Region:     aws.String(region),
		HTTPClient: httpClient,
	}

	if endpoint != "" {
		awsConfig.Endpoint = aws.String(endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true)
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return sess, nil
}
