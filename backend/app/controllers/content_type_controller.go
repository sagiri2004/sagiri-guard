package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/services"
)

type ContentTypeController struct {
	Service *services.ContentTypeService
}

func NewContentTypeController(svc *services.ContentTypeService) *ContentTypeController {
	return &ContentTypeController{Service: svc}
}

func (c *ContentTypeController) List(w http.ResponseWriter, r *http.Request) {
	items, err := c.Service.List()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(items)
}

func (c *ContentTypeController) Create(w http.ResponseWriter, r *http.Request) {
	var body models.ContentType
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	created, err := c.Service.Create(&body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(created)
}

func (c *ContentTypeController) Update(w http.ResponseWriter, r *http.Request) {
	var body models.ContentType
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.ID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	updated, err := c.Service.Update(&body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(updated)
}

func (c *ContentTypeController) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := c.Service.Delete(uint(id)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
