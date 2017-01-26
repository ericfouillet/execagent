package execagent

import "sync"

// Command represents a command to execute.
// The command may be optionally executed asynchronously
type Command struct {
	id      string
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Sync    bool     `json:"sync"`
}

// CommandID represents a command ID executed by the agent
type CommandID struct {
	ID string `json:"id"`
}

// CommandExec represents the execution of a command
type CommandExec struct {
	ID      string   `json:"id"`
	Status  status   `json:"status"`
	Results []string `json:"results"`
}

// A command status
type status string

const (
	success    status = "SUCCESS"
	failure    status = "FAILURE"
	inprogress status = "IN PROGRESS"
)

// Agent is the executing agent
type Agent struct {
	port       int
	mu         sync.Mutex
	executions map[string]CommandExec
	completed  chan CommandExec
}

// AgentRunner is an interface for agent runners
type AgentRunner interface {
	Run() error
}
