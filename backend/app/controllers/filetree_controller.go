package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/middleware"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/services"

	"gorm.io/gorm"
)

type FileTreeController struct {
	Tree *services.FileTreeService
}

func NewFileTreeController(tree *services.FileTreeService) *FileTreeController {
	return &FileTreeController{Tree: tree}
}

func (c *FileTreeController) Sync(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.DeviceID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var changes []dto.FileChange
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := c.Tree.ApplyChanges(claims.DeviceID, changes); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (c *FileTreeController) List(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	claims := middleware.GetClaims(r.Context())
	if deviceID == "" && claims != nil {
		deviceID = claims.DeviceID
	}
	if deviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var parentID *string
	if v, ok := r.URL.Query()["parent_id"]; ok {
		if v[0] != "" {
			parentID = &v[0]
		} else {
			parentID = nil
		}
	}

	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	size := parseIntDefault(r.URL.Query().Get("size"), 50)

	extParam := r.URL.Query().Get("ext")
	var exts []string
	if extParam != "" {
		exts = splitAndTrim(extParam)
	}

	ctParam := r.URL.Query().Get("content_type_id")
	var contentTypeIDs []uint
	if ctParam != "" {
		for _, token := range splitAndTrim(ctParam) {
			if id, err := strconv.Atoi(token); err == nil {
				contentTypeIDs = append(contentTypeIDs, uint(id))
			}
		}
	}

	// Parse include_deleted parameter
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	deletedSince := r.URL.Query().Get("deleted_since") // Format: "6h", "24h", "7d" hoặc timestamp

	query := dto.TreeQuery{
		DeviceUUID:     deviceID,
		ParentID:       parentID,
		Extensions:     exts,
		ContentTypeIDs: contentTypeIDs,
		Search:         r.URL.Query().Get("q"),
		Page:           page,
		PageSize:       size,
		IncludeDeleted: includeDeleted,
		DeletedSince:   deletedSince,
	}

	items, total, err := c.Tree.GetNodes(query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := struct {
		Data     []dto.TreeNodeResponse `json:"data"`
		Total    int64                  `json:"total"`
		Page     int                    `json:"page"`
		PageSize int                    `json:"page_size"`
	}{
		Data:     make([]dto.TreeNodeResponse, 0, len(items)),
		Total:    total,
		Page:     page,
		PageSize: size,
	}

	for _, item := range items {
		resp.Data = append(resp.Data, toTreeNode(item))
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func (c *FileTreeController) AssignContentTypes(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("id")
	if itemID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	claims := middleware.GetClaims(r.Context())
	if deviceID == "" && claims != nil {
		deviceID = claims.DeviceID
	}
	if deviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var body dto.ContentTypeAssignment
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := c.Tree.AssignContentTypes(deviceID, itemID, body.ContentTypeIDs); err != nil {
		if err == gorm.ErrRecordNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func toTreeNode(item *models.Item) dto.TreeNodeResponse {
	nodeType := "folder"
	extension := "folder"
	if item.File != nil {
		nodeType = "file"
		extension = item.File.Extension
	}
	var parentID *string
	if item.ParentID != nil {
		parentID = item.ParentID
	}
	var ctIDs []uint
	for _, ct := range item.ContentTypes {
		ctIDs = append(ctIDs, ct.ID)
	}
	
	var deletedAtUnix *int64
	if item.DeletedAt.Valid {
		unix := item.DeletedAt.Time.Unix()
		deletedAtUnix = &unix
	}
	
	return dto.TreeNodeResponse{
		ID:             item.ID,
		Name:           item.Name,
		Type:           nodeType,
		ParentID:       parentID,
		Size:           item.TotalSize,
		Extension:      extension,
		ContentTypeIDs: ctIDs,
		UpdatedAtUnix:  item.UpdatedAt.Unix(),
		DeletedAtUnix:  deletedAtUnix,
	}
}

func parseIntDefault(value string, def int) int {
	if v, err := strconv.Atoi(value); err == nil && v > 0 {
		return v
	}
	return def
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
