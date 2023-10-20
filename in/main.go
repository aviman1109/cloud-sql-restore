package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
type Version struct {
	ID string `json:"operation_id"`
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

func GetOperationState(input JSONSource) (Operation, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/operations/%s?instance=%s", input.Source.Project, input.Version.OperationID, input.Source.Instance)
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

	var operation Operation
	err = json.NewDecoder(res.Body).Decode(&operation)
	if err != nil {
		return Operation{}, err
	}

	return operation, nil
}

func main() {
	log.SetFlags(0)
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
	for {
		operation, err := GetOperationState(input)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		if operation.Status == "DONE" {
			log.Println("Restore successful!")
			// Convert InsertTime to Taiwan time zone
			taiwanTimeZone, err := time.LoadLocation("Asia/Taipei")
			if err != nil {
				log.Fatal(err)
				os.Exit(1)
			}
			operation.EndTime = operation.EndTime.In(taiwanTimeZone)

			output := Output{
				Version: Version{ID: operation.OperationID},
				Metadata: []Metadata{
					{Name: "backup-id", Value: operation.BackupContext.BackupID},
					{Name: "status", Value: operation.Status},
					{Name: "end-time", Value: operation.EndTime.Format(time.RFC3339)},
					{Name: "type", Value: operation.OperationType},
					{Name: "target-instance", Value: operation.TargetID},
				},
			}
			// print output as JSON
			encodedOutput, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				log.Fatal(err)
				os.Exit(1)
			}
			fmt.Println(string(encodedOutput))
			break
		} else if operation.Status == "RUNNING" || operation.Status == "PENDING" {
			log.Println("Restore state:", operation.Status)
			time.Sleep(30 * time.Second) // Wait 30 seconds before checking again
		} else {
			log.Panicln("Restore state:", operation.Status)
			os.Exit(1)
		}
	}
}
