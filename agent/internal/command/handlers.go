package command

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sagiri-guard/agent/internal/backup"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/state"
	"sagiri-guard/network"
	"strings"
)

type getLogsArg struct {
	Lines int `json:"lines,omitempty"`
}

type getLogsHandler struct{}

func (h getLogsHandler) Kind() Kind { return KindOnce }
func (h getLogsHandler) DecodeArg(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return getLogsArg{Lines: 100}, nil
	}
	var a getLogsArg
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	if a.Lines == 0 {
		a.Lines = 100
	}
	return a, nil
}
func (h getLogsHandler) HandleOnce(arg any) error {
	cfg := config.Get()
	// read log file if configured; fallback to default alongside token path
	logPath := cfg.LogPath
	if logPath == "" {
		logPath = filepath.Join(filepath.Dir(config.TokenFilePath()), "agent.log")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		logger.Errorf("read log failed: %v", err)
		data = []byte("<no log available>")
	}
	// load token & device id from state
	token := strings.TrimSpace(state.GetToken())
	if token == "" {
		logger.Error("no token for posting logs")
		return nil
	}
	deviceID := state.GetDeviceID()
	// backend address
	host, port := config.BackendHTTP()
	q := url.Values{}
	q.Set("deviceid", deviceID)
	path := "/agent/log?" + q.Encode()
	headers := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "text/plain"}
	_, postErr := network.HTTPPostWithHeaders(host, port, path, "text/plain", data, headers)
	if postErr != nil {
		logger.Errorf("post logs failed: %v", postErr)
	}
	return nil
}
func (h getLogsHandler) Start(arg any) (func() error, error) { return nil, nil }

// remaining handlers ...

type backupAutoArg struct {
	IntervalSec int `json:"interval_sec,omitempty"`
}

type backupAutoHandler struct{}

func (h backupAutoHandler) Kind() Kind { return KindStream }
func (h backupAutoHandler) DecodeArg(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return backupAutoArg{IntervalSec: 300}, nil
	}
	var a backupAutoArg
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	if a.IntervalSec == 0 {
		a.IntervalSec = 300
	}
	return a, nil
}
func (h backupAutoHandler) HandleOnce(arg any) error { return nil }
func (h backupAutoHandler) Start(arg any) (func() error, error) {
	logger.Infof("Starting backup_auto with arg=%v", arg)
	// TODO: start background scheduler/monitor for automatic backups
	stop := func() error {
		logger.Info("Stopping backup_auto")
		return nil
	}
	return stop, nil
}

type restoreArg struct {
	FileName string `json:"file_name"`
	DestPath string `json:"dest_path,omitempty"`
}

type restoreHandler struct{}

func (h restoreHandler) Kind() Kind { return KindOnce }
func (h restoreHandler) DecodeArg(raw json.RawMessage) (any, error) {
	var a restoreArg
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
	}
	if a.FileName == "" {
		return nil, fmt.Errorf("missing file_name")
	}
	if a.DestPath == "" {
		a.DestPath = filepath.Join(os.TempDir(), "sagiri-restore", filepath.Base(a.FileName))
	}
	return a, nil
}
func (h restoreHandler) HandleOnce(arg any) error {
	a, ok := arg.(restoreArg)
	if !ok {
		return fmt.Errorf("invalid argument type")
	}
	token := strings.TrimSpace(state.GetToken())
	if token == "" {
		return fmt.Errorf("missing token")
	}
	host, port := config.BackendHTTP()
	session, err := backup.InitDownload(host, port, token, a.FileName)
	if err != nil {
		return err
	}
	if err := backup.DownloadFile(session, a.DestPath); err != nil {
		return err
	}
	logger.Infof("Restored %s to %s", a.FileName, a.DestPath)
	return nil
}
func (h restoreHandler) Start(arg any) (func() error, error) { return nil, nil }

func init() {
	Register("get_logs", getLogsHandler{})
	Register("backup_auto", backupAutoHandler{})
	Register("restore", restoreHandler{})
}
