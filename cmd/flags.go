package cmd

import (
	"bytes"
	"flag"
	"os/exec"
	"strconv"
)

type serverFlags struct {
	fs   *flag.FlagSet
	port *int
	db   *string
}

func newServerFlagSet(name string) *serverFlags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return &serverFlags{
		fs:   fs,
		port: fs.Int("port", 8080, "listen port"),
		db:   fs.String("db", "./mockflow.db", "sqlite database path"),
	}
}

func parseServerFlags(name string, args []string) (int, string, error) {
	flags := newServerFlagSet(name)
	if err := flags.fs.Parse(args); err != nil {
		return 0, "", err
	}
	return *flags.port, *flags.db, nil
}

func runLsof(port int) (string, error) {
	cmd := exec.Command("lsof", "-nP", "-t", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return "", err
	}
	return stdout.String(), nil
}
