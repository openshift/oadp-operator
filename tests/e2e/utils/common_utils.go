package utils

import (
	"io/ioutil"
	"os/exec"
	"strings"
)

func ReadFile(path string) ([]byte, error) {
	file, err := ioutil.ReadFile(path)
	return file, err
}

// Replace new line with carriage return
func ReplaceSecretDataNewLineWithCarriageReturn(data []byte) []byte {
	data = []byte(strings.ReplaceAll(string(data), "\n", "\r\n"))
	return data
}

// Extract tar.gz file to a directory of the same name in the same directory
func ExtractTarGz(pathToDir, tarGzFileName string) error {
	return exec.Command("tar", "-xzf", pathToDir+"/"+tarGzFileName, "-C", pathToDir).Run()
}
