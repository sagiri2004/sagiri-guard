package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type DB struct {
	Host string
	Port int
	User string
	Pass string
	Name string
}

type HTTP struct {
	Host string
	Port int
}

type TCP struct {
	Host string
	Port int
}

type Onedrive struct {
	RefreshToken string
	ClientID     string
	ClientSecret string
	RootFolderID string
	DriveType    string
	DriveID      string
}

type Backup struct {
	StoragePath string
	ChunkSize   int64
	TCP         TCP
}
type Config struct {
	HTTP HTTP
	TCP  TCP
	DB   DB
	JWT  struct {
		Secret string
		Issuer string
		ExpMin int
	}
	Onedrive Onedrive
	Backup   Backup
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Defaults
	v.SetDefault("backend.http.host", "127.0.0.1")
	v.SetDefault("backend.http.port", 9400)
	v.SetDefault("backend.tcp.host", "127.0.0.1")
	v.SetDefault("backend.tcp.port", 9200)
	v.SetDefault("backend.db.host", "127.0.0.1")
	v.SetDefault("backend.db.port", 3306)
	v.SetDefault("backend.db.user", "root")
	v.SetDefault("backend.db.pass", "")
	v.SetDefault("backend.db.name", "sagiri_guard")
	v.SetDefault("backend.onedrive.refresh_token", "")
	v.SetDefault("backend.onedrive.client_id", "")
	v.SetDefault("backend.onedrive.client_secret", "")
	v.SetDefault("backend.onedrive.root_folder_id", "")
	v.SetDefault("backend.onedrive.drive_type", "personal")
	v.SetDefault("backend.onedrive.drive_id", "")
	v.SetDefault("backend.backup.storage_path", "backups")
	v.SetDefault("backend.backup.chunk_size", 524288)
	v.SetDefault("backend.backup.tcp.host", v.GetString("backend.tcp.host"))
	v.SetDefault("backend.backup.tcp.port", v.GetInt("backend.tcp.port")+1)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		HTTP: HTTP{Host: v.GetString("backend.http.host"), Port: v.GetInt("backend.http.port")},
		TCP:  TCP{Host: v.GetString("backend.tcp.host"), Port: v.GetInt("backend.tcp.port")},
		DB:   DB{Host: v.GetString("backend.db.host"), Port: v.GetInt("backend.db.port"), User: v.GetString("backend.db.user"), Pass: v.GetString("backend.db.pass"), Name: v.GetString("backend.db.name")},
		Onedrive: Onedrive{
			RefreshToken: v.GetString("backend.onedrive.refresh_token"),
			ClientID:     v.GetString("backend.onedrive.client_id"),
			ClientSecret: v.GetString("backend.onedrive.client_secret"),
			RootFolderID: v.GetString("backend.onedrive.root_folder_id"),
			DriveType:    v.GetString("backend.onedrive.drive_type"),
			DriveID:      v.GetString("backend.onedrive.drive_id"),
		},
		Backup: Backup{
			StoragePath: v.GetString("backend.backup.storage_path"),
			ChunkSize:   v.GetInt64("backend.backup.chunk_size"),
			TCP: TCP{
				Host: v.GetString("backend.backup.tcp.host"),
				Port: v.GetInt("backend.backup.tcp.port"),
			},
		},
	}
	cfg.JWT.Secret = v.GetString("backend.jwt.secret")
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = "dev-secret"
	}
	cfg.JWT.Issuer = v.GetString("backend.jwt.issuer")
	if cfg.JWT.Issuer == "" {
		cfg.JWT.Issuer = "sagiri-guard"
	}
	cfg.JWT.ExpMin = v.GetInt("backend.jwt.exp_min")
	if cfg.JWT.ExpMin <= 0 {
		cfg.JWT.ExpMin = 60
	}
	return cfg, nil
}
