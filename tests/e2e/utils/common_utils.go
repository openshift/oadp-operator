package utils

import (
	"io/ioutil"
	"os"
	"text/template"
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

// Parses a template file with the provided data and returns contents as byte array
func ParseTemplate(templateFile string, data interface{}) ([]byte, error) {
	// Parse the template file
	var tmpl *template.Template
	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return nil, err
	}

	// Create a temporary file to be able to write parsed data
	tempYamlFile, err := os.CreateTemp("templates/", "temp*.yaml")
	if err != nil {
		return nil, err
	}

	// Write data object values to temporary file
	err = tmpl.Execute(tempYamlFile, data)
	if err != nil {
		return nil, err
	}

	// Read the temporary file as byte array
	parsedData, err := ioutil.ReadFile(tempYamlFile.Name())
	if err != nil {
		return nil, err
	}

	// Close and clean up temp resources
	tempYamlFile.Close()
	defer os.Remove(tempYamlFile.Name())

	return parsedData, nil
}
