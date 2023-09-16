package credentials

import "encoding/json"

type gcpCredAccountKeys string

// From https://github.com/golang/oauth2/blob/d3ed0bb246c8d3c75b63937d9a5eecff9c74d7fe/google/google.go#L95
const (
	serviceAccountKey  gcpCredAccountKeys = "service_account"
	externalAccountKey gcpCredAccountKeys = "external_account"
)

func getGCPSecretAccountTypeKey(secretByte []byte) (gcpCredAccountKeys, error) {
	var f map[string]interface{}
	if err := json.Unmarshal(secretByte, &f); err != nil {
		return "", err
	}
	// following will panic if cannot cast to credAccountKeys
	return gcpCredAccountKeys(f["type"].(string)), nil
}

func gcpSecretAccountTypeIsShortLived(secretName, secretKey, namespace string) (bool, error) {
	secretBytes, err := GetDecodedSecretAsByte(secretName, secretKey, namespace)
	if err != nil {
		return false, err
	}
	credAccountTypeKey, err := getGCPSecretAccountTypeKey(secretBytes)
	if err != nil {
		return false, err
	}
	return credAccountTypeKey == externalAccountKey, nil
}
