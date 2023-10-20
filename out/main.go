package main

import (
	"bytes"
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
	Parameters struct {
		SourceProject  string `json:"source_project"`
		SourceInstance string `json:"source_instance"`
		SourceBackup   string `json:"source_backup"`
	} `json:"params"`
}
type BackupItem struct {
	Kind            string    `json:"kind"`
	Status          string    `json:"status"`
	EnqueuedTime    time.Time `json:"enqueuedTime"`
	BackupID        string    `json:"id"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	Type            string    `json:"type"`
	WindowStartTime time.Time `json:"windowStartTime"`
	Instance        string    `json:"instance"`
	SelfLink        string    `json:"selfLink"`
	Location        string    `json:"location"`
	BackupKind      string    `json:"backupKind"`
}
type BackupRunsList struct {
	Kind  string       `json:"kind"`
	Items []BackupItem `json:"items"`
}
type Operation struct {
	Status        string    `json:"status"`
	OperationID   string    `json:"name"`
	InsertTime    time.Time `json:"insertTime"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	OperationType string    `json:"operationType"`
	TargetID      string    `json:"targetId"`
	BackupContext struct {
		BackupID string `json:"backupId"`
		Kind     string `json:"kind"`
	} `json:"backupContext,omitempty"`
	// Other fields
	// Kind          string    `json:"kind"`
	// TargetLink    string    `json:"targetLink"`
	// SelfLink      string    `json:"selfLink"`
	// TargetProject string    `json:"targetProject"`
	// User string `json:"user,omitempty"`
}
type OperationsList struct {
	Kind  string      `json:"kind"`
	Items []Operation `json:"items"`
}
type PostResponse struct {
	Kind          string    `json:"kind"`
	TargetLink    string    `json:"targetLink"`
	Status        string    `json:"status"`
	User          string    `json:"user"`
	InsertTime    time.Time `json:"insertTime"`
	OperationType string    `json:"operationType"`
	OperationID   string    `json:"name"`
	TargetID      string    `json:"targetId"`
	SelfLink      string    `json:"selfLink"`
	TargetProject string    `json:"targetProject"`
}
type Version struct {
	OperationID string `json:"operation_id"`
}
type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Output struct {
	Version  Version    `json:"version"`
	Metadata []Metadata `json:"metadata"`
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

func ListBackupRuns(input JSONSource) (BackupRunsList, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s/backupRuns", input.Parameters.SourceProject, input.Parameters.SourceInstance)
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return BackupRunsList{}, err
	}

	req.Header.Add("Authorization", getAuthToken())

	res, err := client.Do(req)
	if err != nil {
		return BackupRunsList{}, err
	}
	defer res.Body.Close()

	var backupRun BackupRunsList
	err = json.NewDecoder(res.Body).Decode(&backupRun)
	if err != nil {
		return BackupRunsList{}, err
	}
	return backupRun, nil

}
func sortBackupRuns(backupRuns []BackupItem) (BackupItem, error) {
	if len(backupRuns) == 0 {
		return BackupItem{}, fmt.Errorf("empty backup runs")
	}
	sort.Slice(backupRuns, func(i, j int) bool {
		return backupRuns[i].EnqueuedTime.After(backupRuns[j].EnqueuedTime)
	})
	return backupRuns[0], nil
}
func LatestOperation(input JSONSource) (Operation, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/operations?instance=%s", input.Source.Project, input.Source.Instance)
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return Operation{}, err
	}

	req.Header.Add("Authorization", getAuthToken())

	res, err := client.Do(req)
	if err != nil {
		return Operation{}, err
	}
	defer res.Body.Close()

	var operationsList OperationsList
	err = json.NewDecoder(res.Body).Decode(&operationsList)
	if err != nil {
		return Operation{}, err
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
		return filteredOperations[i].InsertTime.After(filteredOperations[j].InsertTime)
	})

	if len(filteredOperations) == 0 {
		return Operation{}, nil
	}
	return filteredOperations[0], nil
}
func RestoreBackup(input JSONSource, latestBackup BackupItem) (PostResponse, error) {
	// Make HTTP POST request to second API
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s/restoreBackup", input.Source.Project, input.Source.Instance)
	method := "POST"
	requestData := map[string]interface{}{
		"restoreBackupContext": map[string]interface{}{
			"backupRunId": latestBackup.BackupID,
			"project":     input.Parameters.SourceProject,
			"instanceId":  input.Parameters.SourceInstance,
		},
	}
	requestDataBytes, err := json.Marshal(requestData)
	if err != nil {
		return PostResponse{}, err
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, bytes.NewBuffer(requestDataBytes))
	if err != nil {
		return PostResponse{}, err
	}

	req.Header.Add("Authorization", getAuthToken())
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return PostResponse{}, err
	}
	defer res.Body.Close()
	var response PostResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return PostResponse{}, err
	}
	// log.Printf("%+v\n", response)
	return response, nil
}
func main() {
	log.SetFlags(0)
	var input JSONSource
	// Decode JSON from stdin into input struct
	decoder := json.NewDecoder(os.Stdin)
	err := decoder.Decode(&input)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	err = WriteStringToFile(string(input.Source.PrivateKey))
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	var latestBackup BackupItem
	if input.Parameters.SourceBackup == "" {
		backupRuns, err := ListBackupRuns(input)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		latestBackup, err = sortBackupRuns(backupRuns.Items)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		log.Println("Latest backup ID:", latestBackup.BackupID)
	} else {
		// Open the file
		jsonPath := fmt.Sprintf("%s/%s/output.json", os.Args[1], input.Parameters.SourceBackup)
		file, err := os.Open(jsonPath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		// Decode the JSON data
		err = json.NewDecoder(file).Decode(&latestBackup)
		if err != nil {
			log.Fatal(err)
		}

		// Print the backup ID
		log.Println("Imported backup ID:", latestBackup.BackupID)
	}

	latestOperation, err := LatestOperation(input)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if latestOperation.Status == "RUNNING" {
		log.Println("Current instance state:", latestOperation.Status)
		// Convert InsertTime to Taiwan time zone
		taiwanTimeZone, err := time.LoadLocation("Asia/Taipei")
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		latestOperation.InsertTime = latestOperation.InsertTime.In(taiwanTimeZone)

		output := Output{
			Version: Version{OperationID: latestOperation.OperationID},
			Metadata: []Metadata{
				{Name: "status", Value: latestOperation.Status},
				{Name: "insert-time", Value: latestOperation.InsertTime.Format(time.RFC3339)},
				{Name: "type", Value: latestOperation.OperationType},
				{Name: "target-instance", Value: latestOperation.TargetID},
			},
		}

		// print output as JSON
		encodedOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		fmt.Println(string(encodedOutput))

	} else {
		backupState, err := RestoreBackup(input, latestBackup)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		// Convert InsertTime to Taiwan time zone
		taiwanTimeZone, err := time.LoadLocation("Asia/Taipei")
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		backupState.InsertTime = backupState.InsertTime.In(taiwanTimeZone)

		output := Output{
			Version: Version{OperationID: backupState.OperationID},
			Metadata: []Metadata{
				{Name: "backup-id", Value: latestBackup.BackupID},
				{Name: "status", Value: backupState.Status},
				{Name: "insert-time", Value: backupState.InsertTime.Format(time.RFC3339)},
				{Name: "type", Value: backupState.OperationType},
				{Name: "target-instance", Value: backupState.TargetID},
			},
		}

		// print output as JSON
		encodedOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		fmt.Println(string(encodedOutput))
	}
}
