package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/services"
)

type AdminController struct{ Users *services.UserService }

func NewAdminController(users *services.UserService) *AdminController {
	return &AdminController{Users: users}
}

type createUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (c *AdminController) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := c.Users.CreateUser(req.Username, req.Password, req.Role); err != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
