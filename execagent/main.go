package main

import (
	"flag"
	"log"
	"os"

	"github.com/ericfouillet/execagent"
)

const logFile = "./execagent.log"

var port = flag.Int("port", 8086, "The port to use")

func main() {
	flag.Parse()
	f := setupLog()
	defer f.Close()
	agent := execagent.NewAgent(*port)
	agent.RegisterHandlers()
	log.Fatal(agent.Run())
}

func setupLog() *os.File {
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND, os.FileMode(777))
	if err != nil {
		if f, err = os.Create(logFile); err != nil {
			panic(err)
		}
	}
	log.SetOutput(f)
	return f
}
