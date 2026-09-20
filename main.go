package main

import (
	"fmt"
	"os"

	"mockflow/cmd"
)

var Version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "start":
		return cmd.Start(webFS, args[1:])
	case "restart":
		return cmd.Restart(webFS, args[1:])
	case "stop":
		return cmd.Stop(args[1:])
	case "import":
		return cmd.Import(args[1:])
	case "export":
		return cmd.Export(args[1:])
	case "version", "-v", "--version":
		fmt.Printf("mockflow %s\n", Version)
		return nil
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Print(`MockFlow — time-based mock API server

Usage:
  mockflow start [--port 8080] [--db ./mockflow.db]
  mockflow restart [--port 8080] [--db ./mockflow.db]
  mockflow stop [--port 8080]
  mockflow export <file.yaml>
  mockflow import <file.yaml>
  mockflow version
`)
}
