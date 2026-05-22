package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/horizon6666/max-team/internal/agent"
	"github.com/horizon6666/max-team/internal/audit"
	"github.com/horizon6666/max-team/internal/bus"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/llm"
	"github.com/horizon6666/max-team/internal/task"
	"github.com/horizon6666/max-team/internal/tool"
)

//go:embed static
var staticFS embed.FS

type Deps struct {
	Bus      *bus.MessageBus
	TaskMgr  *task.Manager
	Agents   []agent.Agent
	Audit    *audit.Logger
	Router   *llm.Router
	Registry *tool.Registry
	Cfg      *config.Config
	AgentsCfg *config.AgentsConfig
}

type Server struct {
	deps   Deps
	hub    *Hub
	server *http.Server
}

func NewServer(deps Deps) *Server {
	s := &Server{
		deps: deps,
		hub:  NewHub(),
	}
	return s
}

func (s *Server) Start(port int) error {
	s.deps.Bus.AddObserver(func(msg bus.Message) {
		s.hub.BroadcastMessage(msg)
	})

	go s.hub.Run()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/agents", s.handleGetAgents)
	mux.HandleFunc("GET /api/tasks", s.handleGetTasks)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("POST /api/approval", s.handleApproval)
	mux.HandleFunc("GET /api/audit", s.handleGetAudit)
	mux.HandleFunc("GET /api/tools", s.handleGetTools)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("GET /api/stats", s.handleGetStats)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticContent)))

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("[web] starting server on :%d", port)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[web] server error: %v", err)
		}
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.hub.Stop()
	return s.server.Shutdown(ctx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
