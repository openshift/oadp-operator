package utils

import (
	"io/ioutil"
	"strings"
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

func ReplaceSecretDataNewLineWithCarriageReturn(data []byte) []byte {
	// Replace new line with carriage return
	data = []byte(strings.ReplaceAll(string(data), "\n", "\r\n"))
	return data
}
