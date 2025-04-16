package utils

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Replace carriage returns in secret data
func ReplaceCarriageReturn(data map[string][]byte, log logr.Logger) map[string][]byte {
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

// Fetch a provider credential secret
func GetProviderSecret(secretName, namespace string, k8sClient client.Client, ctx context.Context, log logr.Logger) (corev1.Secret, error) {

	secret := corev1.Secret{}
	key := types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}

	err := k8sClient.Get(ctx, key, &secret)
	if err != nil {
		log.Error(err, "failed to get secret from cluster")
		return secret, err
	}

	if secret.Data == nil {
		return secret, fmt.Errorf("secret %s has no data", secretName)
	}

	secret.Data = ReplaceCarriageReturn(secret.Data, log)
	return secret, nil
}


// Parse AWS credential content from secret
func ParseAWSSecret(secret corev1.Secret, secretKey, matchProfile string, log logr.Logger) (string, string, error) {
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

// Extract value to the right of = sign
func getMatchedKeyValue(key, line string) (string, error) {
	line = strings.ReplaceAll(line, " ", "")
	parts := strings.Split(line, "=")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid credential line: %s", line)
	}
	return strings.Trim(parts[1], `"'`), nil
}
