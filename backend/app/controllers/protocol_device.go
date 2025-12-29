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
	ds, err := c.Devices.ListAll()
	if err != nil {
		return nil, err
	}
	out := make([]dto.DeviceSummary, 0, len(ds))
	for _, d := range ds {
		out = append(out, dto.DeviceSummary{
			UUID:   d.UUID,
			Name:   d.Name,
			Online: c.Hub != nil && c.Hub.IsOnline(d.UUID),
		})
	}
	return out, nil
}

func (c *ProtocolController) handleAdminListOnline(excludeDeviceID string) any {
	if c.Hub == nil || c.Devices == nil {
		return []string{}
	}
	ds, err := c.Devices.ListAll()
	if err != nil {
		global.Logger.Warn().Err(err).Msg("list devices failed for admin_list_online")
		return []string{}
	}
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.UUID == excludeDeviceID {
			continue
		}
		if c.Hub.IsOnline(d.UUID) {
			out = append(out, d.UUID)
		}
	}
	return out
}

func (c *ProtocolController) handleAdminListTree(payload json.RawMessage) (any, error) {
	if c.Tree == nil {
		return nil, errors.New("file tree service not available")
	}
	var req dto.AdminListTreeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if req.DeviceID == "" {
		return nil, errors.New("missing device_id")
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 15
	}
	if pageSize > 30 {
		pageSize = 30 // hard cap to keep ACK small
	}

	nodes, _, err := c.Tree.GetNodes(dto.TreeQuery{
		DeviceUUID: req.DeviceID,
		ParentID:   req.ParentID,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		return nil, err
	}
	truncated := false
	if len(nodes) > pageSize {
		nodes = nodes[:pageSize]
		truncated = true
	}
	out := make([]dto.TreeNodeResponse, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, dto.TreeNodeResponse{
			ID:             n.ID,
			Name:           n.Name,
			Type:           nodeType(n),
			ParentID:       n.ParentID,
			Size:           n.TotalSize,
			Extension:      extFromItem(n),
			ContentTypeIDs: contentTypeIDs(n),
			UpdatedAtUnix:  n.UpdatedAt.Unix(),
			DeletedAtUnix:  deletedAtUnix(n),
		})
	}
	return dto.AdminListTreeResponse{Nodes: out, Truncated: truncated}, nil
}

func nodeType(n *models.Item) string {
	if n.FolderID != nil {
		return "dir"
	}
	return "file"
}

func extFromItem(n *models.Item) string {
	if n.File != nil && n.File.Extension != "" {
		return n.File.Extension
	}
	return n.Name
}

func contentTypeIDs(n *models.Item) []uint {
	if len(n.ContentTypes) == 0 {
		return nil
	}
	out := make([]uint, 0, len(n.ContentTypes))
	for _, ct := range n.ContentTypes {
		out = append(out, ct.ID)
	}
	return out
}

func deletedAtUnix(n *models.Item) *int64 {
	if n.DeletedAt.Valid {
		ts := n.DeletedAt.Time.Unix()
		return &ts
	}
	return nil
}
