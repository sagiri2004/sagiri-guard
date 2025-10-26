package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"sagiri-guard/backend/app/services"
)

type AgentLogController struct{ Logs *services.AgentLogService }

func NewAgentLogController(s *services.AgentLogService) *AgentLogController {
	return &AgentLogController{Logs: s}
}

func (c *AgentLogController) Post(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	b, _ := io.ReadAll(r.Body)
	if len(b) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := c.Logs.Create(deviceID, string(b)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (c *AgentLogController) GetLatest(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	logs, err := c.Logs.Latest(deviceID, 1)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(logs)
}
