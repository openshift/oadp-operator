package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/google/uuid"
)

func ReadFile(path string) ([]byte, error) {
	// pass in aws credentials by cli flag
	// from cli:  -cloud=<"filepath">
	// go run main.go -cloud="/Users/emilymcmullan/.aws/credentials"
	// cloud := flag.String("cloud", "", "file path for aws credentials")
	// flag.Parse()
	// save passed in cred file as []byteq
	file, err := ioutil.ReadFile(path)
	return file, err
}

func decodeJson(data []byte) (map[string]interface{}, error) {
	// Return JSON from buffer data
	var jsonData map[string]interface{}

	err := json.Unmarshal(data, &jsonData)
	return jsonData, err
}

func GetJsonData(path string) (map[string]interface{}, error) {
	// Return buffer data for json
	jsonData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeJson(jsonData)
}

func WriteFile(credFile string, data []byte) error {
	err := ioutil.WriteFile(credFile, data, 0644)
	return err
}

func GenNameUuid(prefix string) string {
	uid, _ := uuid.NewUUID()
	return fmt.Sprintf("%s-%s", prefix, uid.String())
}
