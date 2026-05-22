package web

import (
	"encoding/json"
	"net/http"

	"github.com/horizon6666/max-team/internal/bus"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	var agents []map[string]any
	for _, a := range s.deps.Agents {
		st := a.Status()
		agents = append(agents, map[string]any{
			"name":         st.Name,
			"role":         st.Role,
			"status":       st.Status,
			"current_task": st.CurrentTask,
			"provider":     st.Provider,
			"model":        st.Model,
		})
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.deps.TaskMgr.AllTasks()
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t := s.deps.TaskMgr.Get(id)
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	s.deps.Bus.Send(bus.Message{
		From:    "user",
		To:      "max",
		Type:    bus.MsgUserInput,
		Payload: req.Message,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MessageID string `json:"message_id"`
		Approved  bool   `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.deps.Bus.Send(bus.Message{
		From:    "user",
		To:      "scheduler",
		Type:    bus.MsgApprovalResp,
		Payload: req.Approved,
		ReplyTo: req.MessageID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleGetTools(w http.ResponseWriter, r *http.Request) {
	var tools []map[string]string
	for _, a := range s.deps.Agents {
		st := a.Status()
		tools = append(tools, map[string]string{
			"agent": st.Name,
			"role":  st.Role,
		})
	}

	allTools := s.deps.Registry.All()
	var toolList []map[string]string
	for _, t := range allTools {
		toolList = append(toolList, map[string]string{
			"name":        t.Name(),
			"description": t.Description(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tools":  toolList,
		"agents": tools,
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := map[string]any{
		"server": map[string]any{
			"port": s.deps.Cfg.Server.Port,
			"mode": s.deps.Cfg.Server.Mode,
		},
		"llm": map[string]any{
			"default_provider": s.deps.Cfg.LLM.DefaultProvider,
			"default_model":    s.deps.Cfg.LLM.DefaultModel,
		},
		"agents": s.deps.AgentsCfg.Agents,
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	agentFilter := query.Get("agent")
	typeFilter := query.Get("type")
	limit := 100

	events := ReadAuditLog(s.deps.Cfg.Audit.Output, agentFilter, typeFilter, limit)
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := ComputeStats(s.deps.Cfg.Audit.Output)
	writeJSON(w, http.StatusOK, stats)
}
