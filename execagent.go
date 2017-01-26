package execagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// NewAgent creates a new agent on the given port
func NewAgent(port int) *Agent {
	return &Agent{
		port:       port,
		executions: make(map[string]CommandExec),
		completed:  make(chan CommandExec, 5),
	}
}

// RegisterHandlers registers http handlers for the agent.
func (a *Agent) RegisterHandlers() {
	http.HandleFunc("/exec", a.execHandler)
	http.HandleFunc("/status/", a.statusHandler)
	http.HandleFunc("/findandstop", a.findAndStopHandler)
}

// Run starts listening for command completions and runs the server
func (a *Agent) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.collectResults(ctx)
	return http.ListenAndServe(":"+strconv.Itoa(a.port), nil)
}

func (a *Agent) collectResults(ctx context.Context) {
	for {
		select {
		case res := <-a.completed:
			a.mu.Lock()
			a.executions[res.ID] = res
			a.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// execHandler handles the requests to execute a command
func (a *Agent) execHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Only PUT or POST methods are supported", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var cmd Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := a.registerNewCmd()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	go a.execute(ctx, &cmd, id)
	if cmd.Sync {
		defer cancel() // Can not cancel if the command is async
		var completed bool
		var res CommandExec
		for !completed {
			a.mu.Lock()
			res = a.executions[id]
			completed = res.Status != inprogress
			a.mu.Unlock()
			time.Sleep(1 * time.Second)
		}
		if err := json.NewEncoder(w).Encode(&res); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		return
	}
	res := CommandID{ID: id}
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		cancel()
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

// registerNewCmd creates a new command ID and registers it with the agent
func (a *Agent) registerNewCmd() string {
	ok := true
	var idStr string
	a.mu.Lock()
	defer a.mu.Unlock()
	for id := rand.Int(); ok; id = rand.Int() {
		idStr = "cmd-" + strconv.Itoa(id)
		_, ok = a.executions[idStr]
	}
	exec := CommandExec{ID: idStr, Status: inprogress}
	a.executions[idStr] = exec
	return idStr
}

// execute executes the command
func (a *Agent) execute(ctx context.Context, cmd *Command, id string) {
	execute := exec.CommandContext(ctx, cmd.Command, cmd.Args...)
	log.Println("Executing command", cmd.Command, "with arguments", execute.Args)
	out, err := execute.Output()
	if err != nil {
		a.completed <- CommandExec{ID: id, Status: failure, Results: []string{err.Error()}}
		return
	}
	buf := bytes.NewBuffer(out)
	s := bufio.NewScanner(buf)
	var res []string
	for s.Scan() {
		res = append(res, s.Text())
	}
	a.completed <- CommandExec{ID: id, Status: success, Results: res}
}

// findAndStopHandler finds a process by name and tries to stop it
// Use wisely: all processes containing the same name will be stopped
func (a *Agent) findAndStopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Only PUT or POST methods are supported", http.StatusMethodNotAllowed)
		return
	}
	var cmd Command
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "", http.StatusBadRequest)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.findAndStop(ctx, cmd.Command); err != nil {
		http.Error(w, "Could not stop process "+err.Error(), http.StatusBadRequest)
		return
	}
	res := CommandExec{Results: []string{"Process " + cmd.Command + " was stopped"}}
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

func (a *Agent) findAndStop(ctx context.Context, procName string) error {
	cmd, args := getPsCommand(procName)
	rc := exec.CommandContext(ctx, cmd, args...)
	out, err := rc.Output()
	if err != nil {
		return fmt.Errorf("Failed to execute command to find process %v: %v", procName, err.Error())
	}
	buf := bytes.NewBuffer(out)
	s := bufio.NewScanner(buf)
	var res string
	if s.Scan() {
		res = s.Text()
	}
	pid, err := strconv.Atoi(strings.TrimSpace(res))
	if err != nil {
		return fmt.Errorf("Failed to parse process ID for process %v: %v", procName, err.Error())
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("Failed to find process with name %v (PID: %v): %v", procName, pid, err.Error())
	}
	if err = proc.Kill(); err != nil {
		return fmt.Errorf("Failed to stop process %v (PID:%v): %v", procName, pid, err.Error())
	}
	return nil
}

// statusHandler gets the current status of a command
func (a *Agent) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is supported", http.StatusMethodNotAllowed)
		return
	}
	idx := strings.LastIndex(r.URL.Path, "/")
	if idx == -1 {
		http.Error(w, "Invalid request, missing command ID", http.StatusBadRequest)
		return
	}
	id := r.URL.Path[idx+1:]
	a.mu.Lock()
	defer a.mu.Unlock()
	exec, ok := a.executions[id]
	if !ok {
		http.Error(w, "Unknown command ID "+id, http.StatusBadRequest)
		return
	}
	if err := json.NewEncoder(w).Encode(&exec); err != nil {
		http.Error(w, "Issue while retrieving command status for ID: "+id, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}
