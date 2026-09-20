package cmd

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"runtime"

	"mockflow/internal/server"
	"mockflow/internal/store"
)

func Start(webFS fs.FS, args []string) error {
	flags := newServerFlagSet("start")
	if err := flags.fs.Parse(args); err != nil {
		return err
	}
	port := *flags.port
	dbPath := *flags.db

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	n, err := st.CountEndpoints()
	if err != nil {
		return err
	}
	projects, err := st.ListProjects()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("Config UI:        %s/_ui\n", url)
	for _, lan := range lanBaseURLs(port) {
		fmt.Printf("LAN UI:           %s/_ui\n", lan)
	}
	for _, p := range projects {
		fmt.Printf("Project %-12s mock http://localhost:%d (%d endpoints)\n", p.Name, p.Port, p.EndpointCount)
	}
	fmt.Printf("Loaded %d mock endpoints from %s\n", n, dbPath)

	ui := webFS
	if sub, err := fs.Sub(webFS, "web/dist"); err == nil {
		ui = sub
	}

	hub := server.NewHub(st, ui, dbPath, port)
	go openBrowser(url + "/_ui")
	return hub.Listen()
}

func lanBaseURLs(port int) []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			u := fmt.Sprintf("http://%s:%d", ip.String(), port)
			if seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
	}
}
