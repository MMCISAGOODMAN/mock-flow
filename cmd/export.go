package cmd

import (
	"fmt"
	"os"

	"mockflow/internal/config"
	"mockflow/internal/store"
)

func Export(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mockflow export <file.yaml>")
	}
	st, err := store.Open("./mockflow.db")
	if err != nil {
		return err
	}
	defer st.Close()
	eps, err := st.ListProjectsWithEndpoints()
	if err != nil {
		return err
	}
	data, err := config.Export(eps)
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[0], data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", args[0], err)
	}
	n := 0
	for _, p := range eps {
		n += len(p.Endpoints)
	}
	fmt.Printf("Exported %d projects / %d endpoints to %s\n", len(eps), n, args[0])
	return nil
}
