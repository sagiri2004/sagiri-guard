package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type AppConfig struct {
	BackendHost string
	BackendHTTP int
	BackendTCP  int
	TokenPath   string
	LogPath     string
	OsqueryPath string
}

var cfg AppConfig

func Init() AppConfig {
	defaultTokenDir := filepath.Join(os.TempDir(), "sagiri-guard")
	defaultToken := filepath.Join(defaultTokenDir, "agent.token")

	v := viper.New()
	v.SetConfigFile("config/config.yaml")
	v.SetConfigType("yaml")

	// defaults
	v.SetDefault("agent.backend.host", "127.0.0.1")
	v.SetDefault("agent.backend.http", 9400)
	v.SetDefault("agent.backend.tcp", 9200)
	v.SetDefault("agent.token_path", defaultToken)

	_ = v.ReadInConfig()

	cfg = AppConfig{
		BackendHost: v.GetString("agent.backend.host"),
		BackendHTTP: v.GetInt("agent.backend.http"),
		BackendTCP:  v.GetInt("agent.backend.tcp"),
		TokenPath:   v.GetString("agent.token_path"),
		LogPath:     v.GetString("agent.log_path"),
		OsqueryPath: v.GetString("agent.osquery_path"),
	}
	return cfg
}

func Get() AppConfig { return cfg }

func TokenFilePath() string {
	if cfg.TokenPath == "" {
		return filepath.Join(os.TempDir(), "sagiri-guard", "agent.token")
	}
	return cfg.TokenPath
}

func BackendHTTP() (string, int) { return cfg.BackendHost, cfg.BackendHTTP }

func BackendAddr() string { return fmt.Sprintf("%s:%d", cfg.BackendHost, cfg.BackendTCP) }
