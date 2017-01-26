package execagent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestExec(t *testing.T) {
	var b bytes.Buffer
	c := Command{Command: "echo", Args: []string{"hello"}, Sync: true}
	if err := json.NewEncoder(&b).Encode(&c); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "/exec", &b)
	if err != nil {
		t.Fatal(err)
	}

	agent := NewAgent(8080)
	hr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agent.collectResults(ctx)
	handler := http.HandlerFunc(agent.execHandler)
	handler.ServeHTTP(hr, req)
	var cmdID CommandID
	if err := json.NewDecoder(hr.Body).Decode(&cmdID); err != nil {
		t.Fatal(err)
	}
	if cmdID.ID == "" {
		t.Fatal("Should have received an ID")
	}

	req, err = http.NewRequest("GET", "/status/"+cmdID.ID, nil)
	handler = http.HandlerFunc(agent.statusHandler)
	handler.ServeHTTP(hr, req)
	var cmdStatus CommandExec
	if err := json.NewDecoder(hr.Body).Decode(&cmdStatus); err != nil {
		t.Fatal(err)
	}
	if cmdStatus.Status != success {
		t.Fatal("Command should be successful")
	}
}

func TestStop(t *testing.T) {
	// Prepare command to run
	cmd := exec.Command("tail", "-f", "execagent_test.go")
	if err := cmd.Start(); err != nil {
		t.Fatal("Could not start test command")
	}

	// Prepare request
	var b bytes.Buffer
	c := Command{Command: "tail", Args: []string{}, Sync: true}
	if err := json.NewEncoder(&b).Encode(&c); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "/findandstop", &b)
	if err != nil {
		t.Fatal(err)
	}

	agent := NewAgent(8080)
	hr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agent.collectResults(ctx)
	handler := http.HandlerFunc(agent.findAndStopHandler)
	handler.ServeHTTP(hr, req)
	var cmdExec CommandExec
	if err := json.NewDecoder(hr.Body).Decode(&cmdExec); err != nil {
		t.Fatal(err)
	}
	if len(cmdExec.Results) == 0 {
		t.Fatal("No result was received")
	}
	if cmdExec.Results[0] != "Process tail was stopped" {
		t.Fatal("Unexpected result " + cmdExec.Results[0])
	}
}

func TestInvalidStatus(t *testing.T) {
	agent := NewAgent(8080)
	hr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/status/invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(agent.statusHandler)
	handler.ServeHTTP(hr, req)
	if hr.Code != http.StatusBadRequest {
		t.Fatalf("Unexpected status %v", hr.Code)
	}
}
