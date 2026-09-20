package cmd

import (
	"fmt"
	"os"

	"mockflow/internal/config"
	"mockflow/internal/store"
)

func Import(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mockflow import <file.yaml>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read %s: %w", args[0], err)
	}
	projects, err := config.Import(data)
	if err != nil {
		return err
	}
	st, err := store.Open("./mockflow.db")
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.ReplaceAll(projects); err != nil {
		return err
	}
	n := 0
	for _, p := range projects {
		n += len(p.Endpoints)
	}
	fmt.Printf("Imported %d projects / %d endpoints from %s into ./mockflow.db\n", len(projects), n, args[0])
	return nil
}
