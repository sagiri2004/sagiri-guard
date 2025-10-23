package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/middleware"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/services"
)

type DeviceController struct{ Devices *services.DeviceService }

func NewDeviceController(devices *services.DeviceService) *DeviceController {
	return &DeviceController{Devices: devices}
}

type DeviceRequest struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
	Hostname  string `json:"hostname"`
	Arch      string `json:"arch"`
}

func (c *DeviceController) GetByUUID(w http.ResponseWriter, r *http.Request) {
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var d *models.Device
	if dd, err := c.Devices.FindByUUID(uuid); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	} else {
		d = dd
	}
	_ = json.NewEncoder(w).Encode(d)
}

func (c *DeviceController) RegisterOrUpdate(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var req DeviceRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.UUID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	d := models.Device{UUID: req.UUID, Name: req.Name, OSName: req.OSName, OSVersion: req.OSVersion, Hostname: req.Hostname, Arch: req.Arch, UserID: uint(claims.UserID)}
	_ = c.Devices.UpsertDevice(&d)
	_ = json.NewEncoder(w).Encode(d)
}
