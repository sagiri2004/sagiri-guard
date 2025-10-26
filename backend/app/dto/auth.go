package dto

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}
