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
	TreeRepo *repo.FileTreeRepository
	Hub      *socket.Hub
}

func NewAdminBackupController(versions *repo.BackupVersionRepository, treeRepo *repo.FileTreeRepository, hub *socket.Hub) *AdminBackupController {
	return &AdminBackupController{
		Versions: versions,
		TreeRepo: treeRepo,
		Hub:      hub,
	}
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
			FileID:      v.FileID,
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

// ListVersionsByFileID liệt kê tất cả các version của một file theo file_id.
// GET /admin/backup/versions/by-file-id?deviceid=...&file_id=...
func (c *AdminBackupController) ListVersionsByFileID(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	fileID := r.URL.Query().Get("file_id")
	if deviceID == "" || fileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	versions, err := c.Versions.ListByFileID(deviceID, fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]dto.BackupVersionResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, dto.BackupVersionResponse{
			ID:          v.ID,
			DeviceID:    v.DeviceID,
			FileID:      v.FileID,
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

// GetLatestVersionByFileID trả về version mới nhất của một file theo file_id.
// GET /admin/backup/versions/by-file-id/latest?deviceid=...&file_id=...
func (c *AdminBackupController) GetLatestVersionByFileID(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	fileID := r.URL.Query().Get("file_id")
	if deviceID == "" || fileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	version, err := c.Versions.GetLatestByFileID(deviceID, fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if version == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	out := dto.BackupVersionResponse{
		ID:          version.ID,
		DeviceID:    version.DeviceID,
		LogicalPath: version.LogicalPath,
		FileName:    version.FileName,
		StoredName:  version.StoredName,
		Version:     version.Version,
		Size:        version.Size,
		CreatedAt:   version.CreatedAt.Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Restore is deprecated. Use /admin/command endpoint instead.
// Admin should:
// 1. Get versions via GET /admin/backup/versions?deviceid=...&logical_path=...
// 2. Send restore command via POST /admin/command with:
//    {
//      "deviceid": "...",
//      "command": "restore",
//      "argument": {
//        "file_name": "...",      // from stored_name in version response
//        "logical_path": "...",   // optional
//        "dest_path": "..."      // optional
//      }
//    }
//
// DEPRECATED: This method is no longer used. Restore commands are now sent via /admin/command endpoint.
// func (c *AdminBackupController) Restore(w http.ResponseWriter, r *http.Request) {
// 	var req dto.BackupRestoreRequest
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		w.WriteHeader(http.StatusBadRequest)
// 		return
// 	}
// 	if req.DeviceID == "" || req.LogicalPath == "" {
// 		w.WriteHeader(http.StatusBadRequest)
// 		return
// 	}
// 	version, err := c.Versions.Get(req.DeviceID, req.LogicalPath, req.Version)
// 	if err != nil {
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	if version == nil {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
//
// 	// Build restore command for agent
// 	restoreArg := map[string]interface{}{
// 		"file_name":    version.StoredName,
// 		"logical_path": req.LogicalPath,
// 	}
// 	if req.DestPath != "" {
// 		restoreArg["dest_path"] = req.DestPath
// 	}
// 	argBytes, err := json.Marshal(restoreArg)
// 	if err != nil {
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
//
// 	cmd := dto.CommandRequest{
// 		DeviceID: req.DeviceID,
// 		Command:  "restore",
// 		Argument: argBytes,
// 	}
// 	payload, err := json.Marshal(cmd)
// 	if err != nil {
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	payload = append(payload, '\n')
// 	if err := c.Hub.Send(req.DeviceID, payload); err != nil {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	w.WriteHeader(http.StatusAccepted)
// }
