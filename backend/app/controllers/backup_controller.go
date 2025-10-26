package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/middleware"
	"sagiri-guard/backend/app/services"
)

type BackupController struct {
	Backup *services.BackupService
	Client *services.StsOnedriveClientService
}

func NewBackupController(backup *services.BackupService, client *services.StsOnedriveClientService) *BackupController {
	return &BackupController{Backup: backup, Client: client}
}

func (c *BackupController) getDeviceIDFromRequest(r *http.Request) (string, bool) {
	if claims := middleware.GetClaims(r.Context()); claims != nil && claims.DeviceID != "" {
		return claims.DeviceID, true
	}
	return "", false
}

func (c *BackupController) GetUploadCredential(w http.ResponseWriter, r *http.Request) {
	deviceId, ok := c.getDeviceIDFromRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "device_id not found in token"})
		return
	}
	cred, err := c.Backup.AssumeRole(deviceId, "put")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(cred)
}

func (c *BackupController) GetAllCurrentFiles(w http.ResponseWriter, r *http.Request) {
	token, _, err := c.Backup.GetAccessTokenFromRefreshToken("get")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	files, err := c.Client.GetAllCurrentFiles(token)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(files)
}

func (c *BackupController) GetVersionByFileId(w http.ResponseWriter, r *http.Request) {
	deviceId, ok := c.getDeviceIDFromRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "device_id not found in token"})
		return
	}
	fileId := r.URL.Query().Get("file_id")
	if fileId == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "file_id is required"})
		return
	}
	token, _, err := c.Backup.GetAccessTokenFromRefreshToken("get")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	versions, err := c.Client.GetVersionByFileId(fileId, deviceId, token)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(versions)
}

func (c *BackupController) GetVersionByFileIdAndVersionId(w http.ResponseWriter, r *http.Request) {
	deviceId, ok := c.getDeviceIDFromRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "device_id not found in token"})
		return
	}
	fileId := r.URL.Query().Get("file_id")
	versionId := r.URL.Query().Get("version_id")
	if fileId == "" || versionId == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "file_id and version_id are required"})
		return
	}
	token, _, err := c.Backup.GetAccessTokenFromRefreshToken("get")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	version, err := c.Client.GetVersionByFileIdAndVersionId(versionId, fileId, deviceId, token)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(version)
}

func (c *BackupController) GetAllCurrentFilesAndVersions(w http.ResponseWriter, r *http.Request) {
	token, _, err := c.Backup.GetAccessTokenFromRefreshToken("get")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	files, err := c.Client.GetAllCurrentFilesAndVersions(token)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(files)
}
