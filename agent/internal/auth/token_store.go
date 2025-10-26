package auth

import "sync/atomic"

var tokenValue atomic.Value // holds string

func SetCurrentToken(t string) { tokenValue.Store(t) }

func GetCurrentToken() string {
	if v := tokenValue.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
