package controllers

import (
	"encoding/json"
	"net/http"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/socket"
)

// AdminBackupController cung cấp API cho admin xem version và gửi lệnh restore tới agent.
type AdminBackupController struct {
	Versions *repo.BackupVersionRepository
	Hub      *socket.Hub
}

func NewAdminBackupController(versions *repo.BackupVersionRepository, hub *socket.Hub) *AdminBackupController {
	return &AdminBackupController{Versions: versions, Hub: hub}
}

// ListVersions liệt kê các version của một file logic trên 1 device.
// GET /admin/backup/versions?deviceid=...&logical_path=...
func (c *AdminBackupController) ListVersions(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	logicalPath := r.URL.Query().Get("logical_path")
	if deviceID == "" || logicalPath == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	versions, err := c.Versions.List(deviceID, logicalPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]dto.BackupVersionResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, dto.BackupVersionResponse{
			ID:          v.ID,
			DeviceID:    v.DeviceID,
			LogicalPath: v.LogicalPath,
			FileName:    v.FileName,
			StoredName:  v.StoredName,
			Version:     v.Version,
			Size:        v.Size,
			CreatedAt:   v.CreatedAt.Unix(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Restore gửi lệnh "restore" tới agent để nó tải về 1 version cụ thể.
// POST /admin/backup/restore  { device_id, logical_path, version?, dest_path? }
func (c *AdminBackupController) Restore(w http.ResponseWriter, r *http.Request) {
	var req dto.BackupRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.DeviceID == "" || req.LogicalPath == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	version, err := c.Versions.Get(req.DeviceID, req.LogicalPath, req.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if version == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Build restore command for agent
	restoreArg := map[string]string{
		"file_name": version.StoredName,
	}
	if req.DestPath != "" {
		restoreArg["dest_path"] = req.DestPath
	}
	argBytes, err := json.Marshal(restoreArg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	cmd := dto.CommandRequest{
		DeviceID: req.DeviceID,
		Command:  "restore",
		Argument: argBytes,
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	payload = append(payload, '\n')
	if err := c.Hub.Send(req.DeviceID, payload); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
