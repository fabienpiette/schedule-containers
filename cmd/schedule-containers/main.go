package main

import (
	"fmt"
	"os"

	"github.com/fabienpiette/schedule-containers/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: schedule-containers <command> [args]")
		fmt.Println("Commands: serve, schedule, containers")
		os.Exit(1)
	}
	cli.Execute()
}
