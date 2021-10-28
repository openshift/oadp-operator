package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

type jsonData struct {
	Products []Product `json:"data"`
}

type Product struct {
	Name     string  `json:"Name"`
	Color    string  `json:"Color"`
	Price    float32 `json:"Price"`
	Quantity int     `json:"Quantity"`
}

var dataToVerify = map[string]string{
	"Name":     "test" + createRandomString(charSet),
	"Color":    "White",
	"Price":    "24",
	"Quantity": "5",
}

const charSet = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func createRandomString(charset string) string {
	newStr := make([]byte, 10)
	for i := range newStr {
		newStr[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(newStr)
}

func isAppReady(url string) wait.ConditionFunc {
	return func() (bool, error) {
		resp, err := http.Get(url)
		if err != nil {
			return false, err
		}
		if resp.StatusCode != 200 {
			return false, fmt.Errorf("error getting data. Got status code %v", resp.StatusCode)
		}
		return true, nil
	}
}

func postProductData(url string, jsonInput map[string]string) (string, error) {
	postBody, err := json.Marshal(jsonInput)
	if err != nil {
		return "", err
	}
	responseBody := bytes.NewBuffer(postBody)

	// add data
	resp, err := http.Post(url, "application/json", responseBody)

	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error posting data. Got status code %v", resp.StatusCode)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	stringBody := string(body)
	fmt.Print(stringBody)
	return stringBody, nil
}

func getProductData(url string, addedName string) bool {
	resp, err := http.Get(url)

	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	} else {
		defer resp.Body.Close()
	}
	// var to hold response body struct
	info := jsonData{}
	err = json.Unmarshal(body, &info)
	if err != nil {
		log.Fatal(err)
	}
	var verifyName string

	// seatch for matching name
	for i := 0; i < len(info.Products); i++ {
		verifyName = info.Products[i].Name
		if verifyName == addedName {

			// verify names match
			fmt.Println("name in db: ", verifyName)
			fmt.Println("given name: ", addedName)
			return true
		}
	}
	return false
}
