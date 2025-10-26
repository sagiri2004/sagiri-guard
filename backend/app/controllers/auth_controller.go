package controllers

import (
	"encoding/json"
	"net/http"
	"sagiri-guard/backend/app/dto"
	jwtutil "sagiri-guard/backend/app/jwt"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/services"
)

type AuthController struct {
	Users   *services.UserService
	Devices *services.DeviceService
	Signer  *jwtutil.Signer
}

func NewAuthController(users *services.UserService, signer *jwtutil.Signer) *AuthController {
	return &AuthController{Users: users, Signer: signer}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"missing credentials"}`))
		return
	}
	u, err := c.Users.ValidateCredentials(req.Username, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
		return
	}
	// Upsert device if provided
	if req.DeviceID != "" && c.Devices != nil {
		// minimal upsert with UUID only
		d := models.Device{UUID: req.DeviceID, UserID: u.ID}
		_ = c.Devices.UpsertDevice(&d)
	}
	// issue token; DeviceID is placed in claims by upper layers if needed later
	token, err := c.Signer.Sign(u.ID, u.Username, u.Role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"token error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.TokenResponse{AccessToken: token})
}
