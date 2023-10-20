package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	auth "golang.org/x/oauth2/google"
)

type JSONSource struct {
	Source struct {
		Project    string `json:"project"`
		Instance   string `json:"instance"`
		PrivateKey string `json:"private_key"`
	} `json:"source"`
	Version struct {
		OperationID string `json:"operation_id"`
	} `json:"version"`
}
type Operation struct {
	Status        string    `json:"status"`
	OperationID   string    `json:"name"`
	EndTime       time.Time `json:"endTime"`
	OperationType string    `json:"operationType"`
	TargetID      string    `json:"targetId"`
	// Other fields
	// Kind          string    `json:"kind"`
	// TargetLink    string    `json:"targetLink"`
	// InsertTime    time.Time `json:"insertTime"`
	// StartTime     time.Time `json:"startTime"`
	// SelfLink      string    `json:"selfLink"`
	// TargetProject string    `json:"targetProject"`
	// BackupContext struct {
	// 	BackupID string `json:"backupId"`
	// 	Kind     string `json:"kind"`
	// } `json:"backupContext,omitempty"`
	// User string `json:"user,omitempty"`
}
type OperationsList struct {
	Kind  string      `json:"kind"`
	Items []Operation `json:"items"`
}
type OperationID struct {
	OperationID string `json:"operation_id"`
}

func WriteStringToFile(key string) error {
	file, err := os.Create("/service-account.json")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(key)
	if err != nil {
		return err
	}
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/service-account.json")
	return nil
}
func getAuthToken() string {
	ctx := context.Background()
	scopes := []string{
		"https://www.googleapis.com/auth/cloud-platform",
	}
	credentials, err := auth.FindDefaultCredentials(ctx, scopes...)
	if err != nil {
		log.Fatal(err)
	}
	token, err := credentials.TokenSource.Token()
	if err != nil {
		log.Fatal(err)
	}

	return (fmt.Sprintf("Bearer %v", string(token.AccessToken)))
}

func ListOperations(input JSONSource) ([]Operation, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/operations?instance=%s", input.Source.Project, input.Source.Instance)
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return []Operation{}, err
	}

	req.Header.Add("Authorization", getAuthToken())

	res, err := client.Do(req)
	if err != nil {
		return []Operation{}, err
	}
	defer res.Body.Close()

	var operationsList OperationsList
	err = json.NewDecoder(res.Body).Decode(&operationsList)
	if err != nil {
		return []Operation{}, err
	}

	// Filter operations with OperationType as RESTORE_VOLUME
	var filteredOperations []Operation
	for _, operation := range operationsList.Items {
		if operation.OperationType == "RESTORE_VOLUME" {
			filteredOperations = append(filteredOperations, operation)
		}
	}

	// Sort operations based on InsertTime in descending order
	sort.Slice(filteredOperations, func(i, j int) bool {
		return filteredOperations[i].EndTime.Before(filteredOperations[j].EndTime)
	})

	return filteredOperations, nil
}
func GetOperation(input JSONSource) ([]Operation, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/operations/%s?instance=%s", input.Source.Project, input.Version.OperationID, input.Source.Instance)
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return []Operation{}, err
	}

	req.Header.Add("Authorization", getAuthToken())

	res, err := client.Do(req)
	if err != nil {
		return []Operation{}, err
	}
	defer res.Body.Close()

	var operation Operation
	err = json.NewDecoder(res.Body).Decode(&operation)
	if err != nil {
		return []Operation{}, err
	}

	// Create a slice of Operation and append the operationRespond to it
	operations := []Operation{}
	operations = append(operations, operation)

	return operations, nil
}

func main() {
	var input JSONSource
	// Decode JSON from stdin into input struct
	decoder := json.NewDecoder(os.Stdin)
	err := decoder.Decode(&input)
	if err != nil {
		log.Fatal(err)
	}

	err = WriteStringToFile(string(input.Source.PrivateKey))
	if err != nil {
		log.Fatal(err)
	}

	var operations []Operation
	if input.Version.OperationID == "" {
		operations, err = ListOperations(input)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		operations, err = GetOperation(input)
		if err != nil {
			log.Fatal(err)
		}
		operations, err = ListOperations(input)
		if err != nil {
			log.Fatal(err)
		}
	}
	// Extract operation IDs and create list with desired format
	operationsList := make([]OperationID, len(operations))
	for i, operation := range operations {
		operationsList[i] = OperationID{operation.OperationID}
	}

	// Encode the slice of BackupID structs to JSON and print it
	output, err := json.MarshalIndent(operationsList, "", "  ")
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}
