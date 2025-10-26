package state

import "sync/atomic"

type appState struct {
	Token    atomic.Value // string
	DeviceID atomic.Value // string
}

var s appState

func SetToken(t string) { s.Token.Store(t) }
func GetToken() string {
	if v := s.Token.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func SetDeviceID(id string) { s.DeviceID.Store(id) }
func GetDeviceID() string {
	if v := s.DeviceID.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
