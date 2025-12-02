package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/socket"
)

type CommandController struct {
	Hub  *socket.Hub
	Repo *repo.AgentCommandRepository
}

func NewCommandController(h *socket.Hub, r *repo.AgentCommandRepository) *CommandController {
	return &CommandController{Hub: h, Repo: r}
}

func (c *CommandController) Post(w http.ResponseWriter, r *http.Request) {
	var req dto.CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || req.Command == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Lưu command vào queue (DB)
	cmd := &models.AgentCommand{
		DeviceID: req.DeviceID,
		Command:  req.Command,
		Kind:     req.Kind,
		Payload:  string(req.Argument),
		Status:   "pending",
	}
	if err := c.Repo.Create(cmd); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Thử gửi ngay nếu agent đang online
	payload, _ := json.Marshal(req)
	payload = append(payload, '\n')
	if err := c.Hub.Send(req.DeviceID, payload); err != nil {
		// device offline: giữ status "pending" trong DB để retry sau
		w.WriteHeader(http.StatusAccepted)
		return
	}
	_ = c.Repo.MarkSent(cmd.ID)
	w.WriteHeader(http.StatusAccepted)
}

func (c *CommandController) Online(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("deviceid")
	w.Header().Set("Content-Type", "application/json")
	// Nếu client truyền deviceid -> giữ lại behavior cũ (check 1 device)
	if id != "" {
		online := c.Hub.IsOnline(id)
		_ = json.NewEncoder(w).Encode(map[string]bool{"online": online})
		return
	}
	// Nếu không có deviceid -> trả về toàn bộ danh sách device đang online
	list := c.Hub.OnlineDevices()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"online_devices": list,
		"count":          len(list),
	})
}

// Queue trả về danh sách command trong queue của 1 client.
// GET /admin/command/queue?deviceid=...&include_sent=true|false
func (c *CommandController) Queue(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("deviceid")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	includeSent := r.URL.Query().Get("include_sent") == "true"
	cmds, err := c.Repo.ListByDevice(id, includeSent)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	type resp struct {
		ID        uint   `json:"id"`
		Command   string `json:"command"`
		Kind      string `json:"kind"`
		Payload   string `json:"payload"`
		Status    string `json:"status"`
		LastError string `json:"last_error"`
		CreatedAt int64  `json:"created_at"`
	}
	out := make([]resp, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, resp{
			ID:        c.ID,
			Command:   c.Command,
			Kind:      c.Kind,
			Payload:   c.Payload,
			Status:    c.Status,
			LastError: c.LastError,
			CreatedAt: c.CreatedAt.Unix(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
