package utils

import (
	"errors"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"
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

// WaitForCheck periodically runs the given boolean function and sleeps until
// either the function returns true or the timeout is reached. Returns an error
// on timeout.
func WaitForCheck(timeout, period time.Duration, checkFunction func() bool, errorMessage string) error {
	timer := time.After(timeout)
	for {
		if check := checkFunction(); check {
			break
		}
		select {
		case <-timer:
			return errors.New(errorMessage)
		default:
			time.Sleep(period)
		}
	}
	return nil
}
