package lib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"strings"
)

type RequestParameters struct {
	ProxyPodParams *ProxyPodParameters // Required, when using K8s proxy container
	RequestMethod  *HTTPMethod
	URL            string
	Payload        *string // Required for POST method
}

type HTTPMethod string

const (
	GET  HTTPMethod = "GET"
	POST HTTPMethod = "POST"
)

func ReadFile(path string) ([]byte, error) {
	file, err := ioutil.ReadFile(path)
	return file, err
}

func RemoveFileIfExists(filePath string) {
	if _, err := os.Stat(filePath); err == nil {
		os.Remove(filePath)
	}
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

// Checks if the payload is actually valid json
func isPayloadValidJSON(payLoad string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(payLoad), &js) == nil
}

// MakeRequest performs an HTTP request with the given parameters and returns the response,
// error message, and any encountered errors.
//
// It can make such request directly or via proxy pod container, so the URL can be also
// reached using internal to k8s service endpoint.
//
// Parameters:
//   - params: RequestParameters struct containing the details of the HTTP request.
//     The struct includes fields like RequestMethod, URL, Payload, and ProxyPodParams.
//
// Returns:
//   - response: The response body as a string in case of a successful HTTP request.
//   - errorResponse: The error response message if the HTTP request encounters an error.
//   - err: An error object indicating any errors that occurred during the HTTP request.
func MakeRequest(params RequestParameters) (string, string, error) {

	var requestMethod HTTPMethod

	// Allowed is only GET and POST, however
	// Request method defaults to GET when not provided
	if params.RequestMethod == nil {
		requestMethod = GET
	} else if *params.RequestMethod == GET || *params.RequestMethod == POST {
		requestMethod = *params.RequestMethod
	} else {
		log.Printf("Invalid Request Method: %s", *params.RequestMethod)
		return "", "", fmt.Errorf("Invalid Request Method: %s", *params.RequestMethod)
	}

	if params.URL == "" {
		errMsg := "URL in a request can not be empty"
		log.Printf(errMsg)
		return "", "", fmt.Errorf(errMsg)
	}

	// Check if the Payload is provided when using POST
	if requestMethod == POST && (params.Payload == nil || *params.Payload == "") {
		errMsg := "Payload is required while performing POST Request"
		log.Printf(errMsg)
		return "", "", fmt.Errorf(errMsg)
	} else if requestMethod == POST {
		if !isPayloadValidJSON(*params.Payload) {
			errMsg := fmt.Sprintf("Invalid JSON payload: %s", *params.Payload)
			fmt.Println(errMsg)
			return "", "", fmt.Errorf(errMsg)
		}
	}

	if params.ProxyPodParams != nil && params.ProxyPodParams.PodName != "" && params.ProxyPodParams.KubeConfig != nil && params.ProxyPodParams.KubeClient != nil && params.ProxyPodParams.Namespace != "" {
		// Make request via Proxy POD
		var curlInProxyCmd string
		if requestMethod == GET {
			log.Printf("Using Proxy pod container: %s", params.ProxyPodParams.PodName)
			curlInProxyCmd = "curl -X GET --silent --show-error " + params.URL
		} else if requestMethod == POST {
			body, err := convertJsonStringToURLParams(*params.Payload)
			if err != nil {
				return "", "", fmt.Errorf("Error while converting parameters: %v", err)
			}
			curlInProxyCmd = fmt.Sprintf("curl -X POST -d %s --silent --show-error %s", body, params.URL)
		}
		return ExecuteCommandInPodsSh(*params.ProxyPodParams, curlInProxyCmd)
	} else {
		var response string
		var errorResponse string
		var err error
		if requestMethod == POST {
			response, errorResponse, err = MakeHTTPRequest(params.URL, requestMethod, *params.Payload)
		} else {
			response, errorResponse, err = MakeHTTPRequest(params.URL, requestMethod, "")
		}
		if err != nil {
			return "", errorResponse, err
		}
		return response, errorResponse, nil
	}

}

// GetInternalServiceEndpointURL constructs the internal service endpoint URI
// for a service in a Kubernetes cluster.
//
// Parameters:
//   - namespace: The namespace of the service.
//   - serviceName: The name of the service.
//   - servicePort: (Optional) The port number of the service. If not provided,
//     the default port 8000 is used.
//
// Returns:
//   - string: The constructed internal service endpoint URI.
func GetInternalServiceEndpointURL(namespace, serviceName string, servicePort ...int) string {
	port := 8000
	if len(servicePort) > 0 {
		port = servicePort[0]
	}

	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// ConvertJsonStringToURLParams takes a JSON string as input and converts it to URL-encoded parameters.
// It returns a string containing the URL-encoded parameters.
//
// Parameters:
//   - payload (string): The JSON string to be converted.
//
// Returns:
//   - string: The URL-encoded parameters.
//   - error: An error, if any, during the conversion process.
//
// Example:
//
//	input:  `{"name": "John", "age": 30, "city": "New York"}`
//	output: `&{name=John&age=30&city=New+York}`
func convertJsonStringToURLParams(payload string) (string, error) {
	var data map[string]interface{}
	err := json.Unmarshal([]byte(payload), &data)
	if err != nil {
		log.Printf("Can not convert JSON string to URL Param: %s", payload)
		return "", err
	}

	params := neturl.Values{}
	for key, value := range data {
		params.Add(key, fmt.Sprintf("%v", value))
	}
	encodedParams := params.Encode()
	log.Printf("Payload encoded parameters: %s", encodedParams)
	return encodedParams, nil
}

// IsURLReachable checks the reachability of an HTTP or HTTPS URL.
//
// Parameters:
//   - url: The URL to check for reachability.
//
// Returns:
//   - bool: True if the URL is reachable, false otherwise.
//   - error: An error, if any, encountered during the HTTP request.
//
// It performs a HEAD request to the specified URL and returns true if
// the request is successful (status code in the 2xx range), indicating
// that the site is reachable. If there is an error during the request
// or if the status code indicates an error, it returns false.
func IsURLReachable(url string) (bool, error) {
	// Attempt to perform a GET request to the specified URL Head
	resp, err := http.Get(url)
	if err != nil {
		// An error occurred during the HTTP request
		return false, err
	}
	defer resp.Body.Close()

	// Check if the response status code indicates success (2xx range)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	// The response status code indicates an error
	return false, fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
}

// MakeHTTPRequest performs an HTTP request with the specified URL, request method, and payload.
// The function supports both GET and POST methods. If the request method is invalid or an error occurs
// during the HTTP request, it returns an error along with the error response body.
//
// Parameters:
//   - url:           The URL for the HTTP request.
//   - requestMethod:string The HTTP request method (e.g., "GET" or "POST").
//   - payload:       The payload for the POST request. It is optional and can be nil for GET requests.
//
// Returns:
//   - string:        The successful response body for a 2xx status code.
//   - string:        The error response body for non-2xx status codes.
//   - error:         An error indicating any issues during the HTTP request or response handling.
func MakeHTTPRequest(url string, requestMethod HTTPMethod, payload string) (string, string, error) {
	var resp *http.Response
	var req *http.Request
	var err error
	var body string

	if requestMethod == GET {
		resp, err = http.Get(url)
	} else if requestMethod == POST {
		body, err = convertJsonStringToURLParams(payload)
		if err != nil {
			return "", "", err
		}
		req, err = http.NewRequest(string(requestMethod), url, strings.NewReader(body))
		if err != nil {
			log.Printf("Error making post request %s", err)
			return "", "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			if resp != nil {
				log.Printf("Response of POST REQUEST  %s", resp.Status)
			}
			return "", "", err
		}

	} else {
		errMsg := fmt.Sprintf("Invalid request method: %s", requestMethod)
		log.Printf(errMsg)
		return "", "", fmt.Errorf(errMsg)
	}

	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", string(body), fmt.Errorf("Error reading response body: %v", err)
		}
		return string(body), "", nil
	}

	// The response status code indicates an error
	// Read the error response body
	responseBody, responseErr := ioutil.ReadAll(resp.Body)
	if responseErr != nil {
		return "", string(responseBody), fmt.Errorf("HTTP request failed with status code %d: %v", resp.StatusCode, responseErr)
	}

	return "", string(responseBody), fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
}
