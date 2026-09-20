package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"mockflow/internal/store"
)

func Restart(webFS fs.FS, args []string) error {
	port, dbPath, err := parseServerFlags("restart", args)
	if err != nil {
		return err
	}
	if err := stopAll(dbPath, port); err != nil {
		return err
	}
	return Start(webFS, args)
}

func Stop(args []string) error {
	port, dbPath, err := parseServerFlags("stop", args)
	if err != nil {
		return err
	}
	return stopAll(dbPath, port)
}

func stopAll(dbPath string, controlPort int) error {
	ports := map[int]struct{}{controlPort: {}}
	st, err := store.Open(dbPath)
	if err == nil {
		if ps, err := st.ListProjects(); err == nil {
			for _, p := range ps {
				ports[p.Port] = struct{}{}
			}
		}
		_ = st.Close()
	}
	var first error
	for p := range ports {
		if err := stopPort(p); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func stopPort(port int) error {
	pids, err := listenPIDs(port)
	if err != nil {
		return err
	}
	self := os.Getpid()
	stopped := 0
	for _, pid := range pids {
		if pid <= 0 || pid == self {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("find pid %d: %w", pid, err)
		}
		if err := proc.Signal(os.Interrupt); err != nil {
			if err := proc.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
				return fmt.Errorf("stop pid %d: %w", pid, err)
			}
		}
		stopped++
		fmt.Printf("Stopped process %d on port %d\n", pid, port)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		left, err := listenPIDs(port)
		if err != nil {
			return err
		}
		busy := false
		for _, pid := range left {
			if pid != self {
				busy = true
				break
			}
		}
		if !busy {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	left, err := listenPIDs(port)
	if err != nil {
		return err
	}
	for _, pid := range left {
		if pid == self {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Kill()
		fmt.Printf("Killed process %d on port %d\n", pid, port)
	}
	if stopped == 0 {
		fmt.Printf("No mockflow server listening on port %d\n", port)
	}
	return nil
}

func listenPIDs(port int) ([]int, error) {
	out, err := runLsof(port)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return nil, fmt.Errorf("need lsof to stop/restart (install lsof or stop the process manually)")
		}
		return nil, fmt.Errorf("list listeners on :%d: %w", port, err)
	}
	var pids []int
	seen := map[int]bool{}
	for _, line := range strings.Fields(out) {
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids, nil
}
