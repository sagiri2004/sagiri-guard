package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/global"
)

type WebsiteBlockController struct {
	service *services.WebsiteBlockService
	hub    *socket.Hub
	cmdRepo *repo.AgentCommandRepository
}

func NewWebsiteBlockController(svc *services.WebsiteBlockService, hub *socket.Hub, cmdRepo *repo.AgentCommandRepository) *WebsiteBlockController {
	return &WebsiteBlockController{service: svc, hub: hub, cmdRepo: cmdRepo}
}

// CreateRule POST /admin/website-block/rule?deviceid=...
func (c *WebsiteBlockController) CreateRule(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing deviceid")
		return
	}

	var req dto.WebsiteBlockRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	rule, err := c.service.CreateRule(deviceID, req)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to create website block rule")
		writeJSONError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

// ListRules GET /admin/website-block/rules?deviceid=...
func (c *WebsiteBlockController) ListRules(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing deviceid")
		return
	}

	rules, err := c.service.ListRules(deviceID)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to list website block rules")
		writeJSONError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}

	writeJSON(w, http.StatusOK, rules)
}

// UpdateRule PUT /admin/website-block/rule?id=...
func (c *WebsiteBlockController) UpdateRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req dto.WebsiteBlockRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if err := c.service.UpdateRule(uint(id), req); err != nil {
		global.Logger.Error().Err(err).Uint64("id", id).Msg("failed to update website block rule")
		writeJSONError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteRule DELETE /admin/website-block/rule?id=...
func (c *WebsiteBlockController) DeleteRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if err := c.service.DeleteRule(uint(id)); err != nil {
		global.Logger.Error().Err(err).Uint64("id", id).Msg("failed to delete website block rule")
		writeJSONError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetStatus GET /admin/website-block/status?deviceid=...
func (c *WebsiteBlockController) GetStatus(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing deviceid")
		return
	}

	status, err := c.service.GetStatus(deviceID)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to get website block status")
		writeJSONError(w, http.StatusInternalServerError, "failed to get status")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// UpdateStatus PUT /admin/website-block/status?deviceid=...
func (c *WebsiteBlockController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing deviceid")
		return
	}

	var req dto.WebsiteBlockStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if err := c.service.UpdateStatus(deviceID, req.Enabled); err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to update website block status")
		writeJSONError(w, http.StatusInternalServerError, "failed to update status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// SyncRules POST /admin/website-block/sync?deviceid=... (gửi command xuống agent)
func (c *WebsiteBlockController) SyncRules(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceid")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing deviceid")
		return
	}

	syncData, err := c.service.GetSyncData(deviceID)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to get sync data")
		writeJSONError(w, http.StatusInternalServerError, "failed to get sync data")
		return
	}

	// Tạo command để gửi xuống agent
	argBytes, err := json.Marshal(syncData)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to marshal sync data")
		writeJSONError(w, http.StatusInternalServerError, "failed to marshal command")
		return
	}

	cmd := &models.AgentCommand{
		DeviceID: deviceID,
		Command:  "block_website",
		Kind:     "once",
		Payload:  string(argBytes),
		Status:   "pending",
	}
	if err := c.cmdRepo.Create(cmd); err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("failed to create block_website command")
		writeJSONError(w, http.StatusInternalServerError, "failed to queue command")
		return
	}

	// Thử gửi ngay nếu agent online
	req := dto.CommandRequest{
		DeviceID: deviceID,
		Command:  "block_website",
		Kind:     "once",
		Argument: argBytes,
	}
	payload, _ := json.Marshal(req)
	payload = append(payload, '\n')
	if err := c.hub.Send(deviceID, payload); err != nil {
		global.Logger.Warn().Err(err).Str("device", deviceID).Msg("agent offline, command queued")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "command queued, agent offline"})
		return
	}

	_ = c.cmdRepo.MarkSent(cmd.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync command sent"})
}

