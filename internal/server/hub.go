package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"mockflow/internal/store"
)

type Hub struct {
	mu          sync.Mutex
	store       *store.Store
	srv         *Server
	controlPort int
	control     *http.Server
	extras      map[int]*http.Server
}

func NewHub(st *store.Store, webFS fs.FS, dbPath string, controlPort int) *Hub {
	h := &Hub{
		store:       st,
		controlPort: controlPort,
		extras:      map[int]*http.Server{},
	}
	h.srv = New(st, webFS, dbPath, controlPort)
	h.srv.hub = h
	h.control = &http.Server{Addr: fmt.Sprintf(":%d", controlPort), Handler: h.srv.Handler()}
	return h
}

func (h *Hub) Server() *Server {
	return h.srv
}

func (h *Hub) Listen() error {
	if err := h.SyncExtra(); err != nil {
		return err
	}
	log.Printf("listening on %s", h.control.Addr)
	return h.control.ListenAndServe()
}

func (h *Hub) SyncExtra() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	projects, err := h.store.ListProjects()
	if err != nil {
		return err
	}
	wanted := map[int]int64{}
	for _, p := range projects {
		if p.Port == h.controlPort {
			continue
		}
		wanted[p.Port] = p.ID
	}

	for port, srv := range h.extras {
		if _, ok := wanted[port]; ok {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		delete(h.extras, port)
		log.Printf("stopped mock listener :%d", port)
	}

	started := []int{}
	for port, pid := range wanted {
		if _, ok := h.extras[port]; ok {
			continue
		}
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			for _, sp := range started {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_ = h.extras[sp].Shutdown(ctx)
				cancel()
				delete(h.extras, sp)
			}
			return fmt.Errorf("listen mock port %d: %w", port, err)
		}
		handler := h.srv.mockHandler(pid)
		hs := &http.Server{Addr: addr, Handler: handler}
		h.extras[port] = hs
		started = append(started, port)
		go func(srv *http.Server, ln net.Listener, port int) {
			log.Printf("mock project listening on :%d", port)
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Printf("mock port %d: %v", port, err)
			}
		}(hs, ln, port)
	}
	return nil
}
