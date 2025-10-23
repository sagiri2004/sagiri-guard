package device

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Info struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
	Hostname  string `json:"hostname"`
	Arch      string `json:"arch"`
}

func Get(baseURL, token, uuid string) (*Info, int, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/devices?uuid=%s", baseURL, uuid), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	var d Info
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, resp.StatusCode, err
	}
	return &d, resp.StatusCode, nil
}

func Register(baseURL, token string, d Info) (*Info, int, error) {
	b, _ := json.Marshal(d)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/devices/register", baseURL), bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, resp.StatusCode, fmt.Errorf("register failed: %s", string(data))
	}
	var out Info
	_ = json.Unmarshal(data, &out)
	return &out, resp.StatusCode, nil
}
