package osquery

import (
	"encoding/json"
	"errors"
	"os/exec"
	"sagiri-guard/agent/internal/config"
)

type SystemInfo struct {
	UUID     string `json:"uuid"`
	Hostname string `json:"hostname"`
	CPUBrand string `json:"cpu_brand"`
	Hardware string `json:"hardware_model"`
}

type OSVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func runJSON(query string, out interface{}) error {
	bin := config.Get().OsqueryPath
	if bin == "" {
		bin = "osqueryi"
	}
	cmd := exec.Command(bin, "--json", query)
	raw, err := cmd.Output()
	if err != nil {
		return err
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return err
	}
	if len(arr) == 0 {
		return errors.New("no rows")
	}
	// re-marshal first row to target struct
	b, _ := json.Marshal(arr[0])
	return json.Unmarshal(b, out)
}

func Collect() (SystemInfo, OSVersion, error) {
	var si SystemInfo
	var osv OSVersion
	if err := runJSON("SELECT uuid, hostname, cpu_brand, hardware_model FROM system_info;", &si); err != nil {
		return si, osv, err
	}
	if err := runJSON("SELECT name, version FROM os_version;", &osv); err != nil {
		return si, osv, err
	}
	return si, osv, nil
}
