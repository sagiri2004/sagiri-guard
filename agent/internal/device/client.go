package device

import (
	"encoding/json"
	"fmt"
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
	resp, err := network.HTTPGetWithHeaders(host, port, "/devices?uuid="+uuid, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, 0, err
	}
	var d Info
	if err := json.Unmarshal([]byte(resp), &d); err != nil {
		return nil, 0, err
	}
	return &d, 200, nil
}

func Register(host string, port int, token string, d Info) (*Info, int, error) {
	b, _ := json.Marshal(d)
	resp, err := network.HTTPPostWithHeaders(host, port, "/devices/register", "application/json", b, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, 0, err
	}
	var out Info
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return nil, 0, fmt.Errorf("register failed: %v | raw=%s", err, resp)
	}
	return &out, 200, nil
}
