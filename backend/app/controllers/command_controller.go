package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/socket"
)

type CommandController struct {
	Hub         *socket.Hub
	Repo        *repo.AgentCommandRepository
	TreeRepo    *repo.FileTreeRepository
	VersionRepo *repo.BackupVersionRepository
}

func NewCommandController(h *socket.Hub, r *repo.AgentCommandRepository, treeRepo *repo.FileTreeRepository, versionRepo *repo.BackupVersionRepository) *CommandController {
	return &CommandController{
		Hub:         h,
		Repo:        r,
		TreeRepo:    treeRepo,
		VersionRepo: versionRepo,
	}
}

func (c *CommandController) Post(w http.ResponseWriter, r *http.Request) {
	var req dto.CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || req.Command == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Xử lý đặc biệt cho restore command: enrich với file_id và path
	if req.Command == "restore" {
		if err := c.enrichRestoreCommand(&req); err != nil {
			http.Error(w, "Failed to enrich restore command: "+err.Error(), http.StatusInternalServerError)
			return
		}
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

// enrichRestoreCommand query file_id và version_id để lấy path và file_name, sau đó enrich vào restore command argument
func (c *CommandController) enrichRestoreCommand(req *dto.CommandRequest) error {
	// Parse argument hiện tại
	var arg map[string]interface{}
	if len(req.Argument) > 0 {
		if err := json.Unmarshal(req.Argument, &arg); err != nil {
			return err
		}
	} else {
		arg = make(map[string]interface{})
	}

	// Lấy file_id và version_id từ argument (bắt buộc)
	fileID, ok := arg["file_id"].(string)
	if !ok || fileID == "" {
		return errors.New("missing file_id in restore command argument")
	}

	// Lấy version_id (có thể là số hoặc string)
	var versionID uint
	switch v := arg["version_id"].(type) {
	case float64:
		versionID = uint(v)
	case int:
		versionID = uint(v)
	case uint:
		versionID = v
	case string:
		// Nếu là string, thử parse
		var parsed uint
		if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil {
			return fmt.Errorf("invalid version_id format: %v", v)
		}
		versionID = parsed
	default:
		return errors.New("missing or invalid version_id in restore command argument")
	}

	// Query FileNode từ file_id (bao gồm soft deleted)
	fileNode, err := c.TreeRepo.GetFileNodeByID(req.DeviceID, fileID)
	if err != nil {
		return fmt.Errorf("failed to query FileNode: %w", err)
	}
	if fileNode == nil {
		return errors.New("file not found for file_id: " + fileID)
	}

	// Query BackupFileVersion từ version_id
	version, err := c.VersionRepo.GetByID(versionID)
	if err != nil {
		return fmt.Errorf("failed to query BackupFileVersion: %w", err)
	}
	if version == nil {
		return fmt.Errorf("backup version not found for version_id: %d", versionID)
	}

	// Verify version thuộc về device và file này
	if version.DeviceID != req.DeviceID {
		return fmt.Errorf("version device_id mismatch: expected %s, got %s", req.DeviceID, version.DeviceID)
	}

	// Thêm file_name (StoredName) và dest_path vào argument
	arg["file_name"] = version.StoredName
	// Thêm path (ưu tiên CurrentPath, fallback OriginPath)
	if fileNode.CurrentPath != "" {
		arg["dest_path"] = fileNode.CurrentPath
	} else if fileNode.OriginPath != "" {
		arg["dest_path"] = fileNode.OriginPath
	} else {
		return errors.New("file has no path (both CurrentPath and OriginPath are empty)")
	}

	// Marshal lại argument
	argBytes, err := json.Marshal(arg)
	if err != nil {
		return err
	}
	req.Argument = argBytes

	return nil
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
