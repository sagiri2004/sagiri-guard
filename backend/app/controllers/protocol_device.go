package controllers

import (
	"encoding/json"
	"errors"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/global"
)

func (c *ProtocolController) handleDeviceRegister(deviceID string, payload json.RawMessage) error {
	if !c.isAuthorized(deviceID) {
		return errors.New("unauthorized")
	}
	if c.Devices == nil {
		return nil
	}
	var req dto.DeviceRequest
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return err
		}
	}
	if req.UUID == "" {
		// fallback to device id from login
		req.UUID = deviceID
	}
	if req.UUID == "" {
		return errors.New("missing uuid")
	}
	d := models.Device{
		UUID:      req.UUID,
		Name:      req.Name,
		OSName:    req.OSName,
		OSVersion: req.OSVersion,
		Hostname:  req.Hostname,
		Arch:      req.Arch,
	}
	if err := c.Devices.UpsertDevice(&d); err != nil {
		return err
	}
	return nil
}

func (c *ProtocolController) handleFileTreeSync(deviceID string, payload json.RawMessage) error {
	if !c.isAuthorized(deviceID) {
		return errors.New("unauthorized")
	}
	if c.Tree == nil {
		return nil
	}
	if deviceID == "" {
		return errors.New("missing device id")
	}
	var changes []dto.FileChange
	if err := json.Unmarshal(payload, &changes); err != nil {
		return err
	}
	if err := c.Tree.ApplyChanges(deviceID, changes); err != nil {
		return err
	}
	return nil
}

func (c *ProtocolController) handleAgentLog(deviceID string, payload json.RawMessage) error {
	if !c.isAuthorized(deviceID) {
		return errors.New("unauthorized")
	}
	if c.Logs == nil {
		return nil
	}
	if deviceID == "" {
		return errors.New("missing device id")
	}
	var body struct {
		Lines string `json:"lines"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return err
	}
	if body.Lines == "" {
		return errors.New("empty lines")
	}
	if err := c.Logs.Create(deviceID, body.Lines); err != nil {
		return err
	}
	global.Logger.Info().Str("device", deviceID).Int("len", len(body.Lines)).Msg("agent log stored")
	return nil
}

func (c *ProtocolController) handleAdminListDevices() (any, error) {
	if c.Devices == nil {
		return nil, errors.New("device service not available")
	}
	onlineSet := map[string]struct{}{}
	if c.Hub != nil {
		for _, id := range c.Hub.OnlineDevices() {
			onlineSet[id] = struct{}{}
		}
	}
	ds, err := c.Devices.ListAll()
	if err != nil {
		return nil, err
	}
	out := make([]dto.DeviceSummary, 0, len(ds))
	for _, d := range ds {
		out = append(out, dto.DeviceSummary{
			UUID:   d.UUID,
			Name:   d.Name,
			Online: has(onlineSet, d.UUID),
		})
	}
	return out, nil
}

func (c *ProtocolController) handleAdminListOnline() any {
	if c.Hub == nil {
		return []string{}
	}
	return c.Hub.OnlineDevices()
}

func has(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}
