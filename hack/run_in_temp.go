package main

import (
	"log"
	"os"
	"os/exec"
)

func aux() int {
	tempDir, err := os.MkdirTemp(os.TempDir(), "tmp.*")
	if err != nil {
		log.Print(err.Error())
		return 1
	}
	if len(tempDir) == 0 {
		log.Print("temporary directory is empty, aborting")
		return 1
	}
	defer func() {
		cmd := exec.Command("chmod", "--recursive", "777", tempDir)
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		err = os.RemoveAll(tempDir)
		if err != nil {
			log.Fatal(err)
		}
	}()

	log.Printf("temporary directory: %v", tempDir)
	pwd, err := os.Getwd()
	if err != nil {
		log.Print(err.Error())
		return 1
	}
	log.Printf("directory to copy into temporary directory: %v", pwd)
	copy := exec.Command("cp", "--recursive", pwd, tempDir)
	err = copy.Run()
	if err != nil {
		log.Print(err.Error())
		return 1
	}

	// cmd := exec.Command("make", "docker-build", "IMG=ttl.sh/oadp-operator-mateus-123456:1h")
	cmd := exec.Command("make", "help")
	cmd.Dir = tempDir
	output, err := cmd.Output()
	if err != nil {
		log.Print(err.Error())
		return 1
	}
	log.Printf("stdout: %s", output)
	return 0
}

func main() {
	os.Exit(aux())
}
