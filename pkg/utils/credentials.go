package utils

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Replace carriage returns in secret data
func ReplaceCarriageReturn(data map[string][]byte) map[string][]byte {
	if data == nil {
		return data
	}

	for k, v := range data {
		if strings.Contains(string(v), "\r\n") {
			data[k] = []byte(strings.ReplaceAll(string(v), "\r\n", "\n"))
		}
	}

	return data
}

// REMOVE ME, Trigger ci
// Fetch a provider credential secret
func GetProviderSecret(secretName, namespace string, k8sClient client.Client, ctx context.Context) (corev1.Secret, error) {

	secret := corev1.Secret{}
	key := types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}

	err := k8sClient.Get(ctx, key, &secret)
	if err != nil {
		return secret, err
	}

	if secret.Data == nil {
		return secret, fmt.Errorf("secret %s has no data", secretName)
	}

	secret.Data = ReplaceCarriageReturn(secret.Data)
	return secret, nil
}

// Parse AWS credential content from secret
func ParseAWSSecret(secret corev1.Secret, secretKey, matchProfile string) (string, string, error) {
	AWSAccessKey, AWSSecretKey := "", ""
	profile := ""
	lines := strings.Split(string(secret.Data[secretKey]), "\n")

	keyNameRegex := regexp.MustCompile(`\[.*\]`)
	accessKeyKey := "aws_access_key_id"
	secretKeyKey := "aws_secret_access_key"
	accessRegex := regexp.MustCompile(`\b` + accessKeyKey + `\b`)
	secretRegex := regexp.MustCompile(`\b` + secretKeyKey + `\b`)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if keyNameRegex.MatchString(line) {
			// Extract profile name
			profileName := strings.Trim(line, "[] ")
			if profileName != matchProfile {
				continue
			}
			profile = profileName

			// Parse credentials
			for _, profLine := range lines[i+1:] {
				if strings.TrimSpace(profLine) == "" {
					continue
				}
				if accessRegex.MatchString(profLine) {
					AWSAccessKey, _ = getMatchedKeyValue(accessKeyKey, profLine)
				} else if secretRegex.MatchString(profLine) {
					AWSSecretKey, _ = getMatchedKeyValue(secretKeyKey, profLine)
				} else {
					break
				}
			}
			break
		}
	}

	if profile == "" || AWSAccessKey == "" || AWSSecretKey == "" {
		return "", "", errors.New("failed to extract AWS credentials or profile from secret")
	}
	return AWSAccessKey, AWSSecretKey, nil
}

// Return value to the right of = sign with quotations and spaces removed.
func getMatchedKeyValue(key string, s string) (string, error) {
	for _, removeChar := range []string{"\"", "'", " "} {
		s = strings.ReplaceAll(s, removeChar, "")
	}
	for _, prefix := range []string{key, "="} {
		s = strings.TrimPrefix(s, prefix)
	}
	if len(s) == 0 {
		return s, errors.New("secret parsing error in key " + key)
	}
	return s, nil
}
