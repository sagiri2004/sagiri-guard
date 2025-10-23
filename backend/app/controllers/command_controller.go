package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/socket"
)

type CommandController struct{ Hub *socket.Hub }

func NewCommandController(h *socket.Hub) *CommandController { return &CommandController{Hub: h} }

func (c *CommandController) Post(w http.ResponseWriter, r *http.Request) {
	var req dto.CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || req.Command == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(req)
	payload = append(payload, '\n')
	if err := c.Hub.Send(req.DeviceID, payload); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *CommandController) Online(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("deviceid")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	online := c.Hub.IsOnline(id)
	_ = json.NewEncoder(w).Encode(map[string]bool{"online": online})
}
