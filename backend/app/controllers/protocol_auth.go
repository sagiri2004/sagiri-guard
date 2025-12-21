package controllers

import (
	"encoding/json"
	"errors"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"

	"github.com/google/uuid"

	"gorm.io/gorm"
)

func (c *ProtocolController) handleLogin(msgDeviceID string, payload json.RawMessage) (any, error) {
	if c.Users == nil || c.Signer == nil {
		return nil, errors.New("auth not available")
	}
	var req dto.ProtocolLoginRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = msgDeviceID
	}
	if deviceID == "" {
		// allow admin/headless login without device id
		deviceID = "admin-" + uuid.NewString()
	}
	user, err := c.Users.ValidateCredentials(req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	token, err := c.Signer.Sign(user.ID, user.Username, user.Role, deviceID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.deviceTokens[deviceID] = token
	if msgDeviceID != "" && msgDeviceID != deviceID {
		// also cache under frame device id to keep connection/device map in sync
		c.deviceTokens[msgDeviceID] = token
	}
	c.mu.Unlock()

	// Ensure device exists and enrich basic info (first login on a machine may not have registered yet)
	if c.Devices != nil && deviceID != "" {
		info := models.Device{
			UUID:      deviceID,
			Name:      req.Name,
			OSName:    req.OSName,
			OSVersion: req.OSVersion,
			Hostname:  req.Hostname,
			Arch:      req.Arch,
		}
		if _, err := c.Devices.FindByUUID(deviceID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_ = c.Devices.UpsertDevice(&info)
			}
		} else {
			_ = c.Devices.UpsertDevice(&info)
		}
	}

	return map[string]string{"token": token, "device_id": deviceID}, nil
}
