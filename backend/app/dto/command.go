package dto

type CommandRequest struct {
	DeviceID string      `json:"deviceid"`
	Command  string      `json:"command"`
	Argument interface{} `json:"argument"`
}
