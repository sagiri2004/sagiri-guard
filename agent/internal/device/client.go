package device

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"sagiri-guard/network"
)

type Info struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
	Hostname  string `json:"hostname"`
	Arch      string `json:"arch"`
}

func Get(host string, port int, token, uuid string) (*Info, int, error) {
	path := "/devices?uuid=" + url.QueryEscape(uuid)
	status, body, err := network.HTTPGetWithHeadersEx(host, port, path, authHeader(token))
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("device get failed: status=%d body=%s", status, body)
	}
	var d Info
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		return nil, status, err
	}
	return &d, status, nil
}

func Register(host string, port int, token string, d Info) (*Info, int, error) {
	b, _ := json.Marshal(d)
	headers := authHeader(token)
	if headers == nil {
		return nil, 0, fmt.Errorf("device register failed: token is empty, cannot create auth header")
	}
	status, body, err := network.HTTPPostWithHeadersEx(host, port, "/devices/register", "application/json", b, headers)
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("device register failed: status=%d body=%s", status, body)
	}
	var out Info
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, status, fmt.Errorf("register failed: %v | raw=%s", err, body)
	}
	return &out, status, nil
}

func authHeader(token string) map[string]string {
	if token == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + token}
}
