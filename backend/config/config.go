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

type TCP struct {
	Host string
	Port int
}

type Backup struct {
	StoragePath string
	ChunkSize   int64
	TCP         TCP
}
type Config struct {
	TCP TCP
	DB  DB
	JWT struct {
		Secret string
		Issuer string
		ExpMin int
	}
	Backup Backup
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Defaults
	v.SetDefault("backend.host", "127.0.0.1")
	v.SetDefault("backend.port", 9200)
	// maintain legacy keys but mirror defaults
	v.SetDefault("backend.tcp.host", v.GetString("backend.host"))
	v.SetDefault("backend.tcp.port", v.GetInt("backend.port"))
	v.SetDefault("backend.db.host", "127.0.0.1")
	v.SetDefault("backend.db.port", 3306)
	v.SetDefault("backend.db.user", "root")
	v.SetDefault("backend.db.pass", "")
	v.SetDefault("backend.db.name", "sagiri_guard")
	v.SetDefault("backend.backup.storage_path", "backups")
	v.SetDefault("backend.backup.chunk_size", 524288) // 512KB
	v.SetDefault("backend.backup.tcp.host", v.GetString("backend.host"))
	v.SetDefault("backend.backup.tcp.port", v.GetInt("backend.port"))
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	host := v.GetString("backend.host")
	if host == "" {
		host = v.GetString("backend.tcp.host")
	}
	port := v.GetInt("backend.port")
	if port == 0 {
		port = v.GetInt("backend.tcp.port")
	}

	backupHost := v.GetString("backend.backup.tcp.host")
	if backupHost == "" {
		backupHost = host
	}
	backupPort := v.GetInt("backend.backup.tcp.port")
	if backupPort == 0 {
		backupPort = port
	}

	cfg := &Config{
		TCP: TCP{Host: host, Port: port},
		DB:  DB{Host: v.GetString("backend.db.host"), Port: v.GetInt("backend.db.port"), User: v.GetString("backend.db.user"), Pass: v.GetString("backend.db.pass"), Name: v.GetString("backend.db.name")},
		Backup: Backup{
			StoragePath: v.GetString("backend.backup.storage_path"),
			ChunkSize:   v.GetInt64("backend.backup.chunk_size"),
			TCP: TCP{
				Host: backupHost,
				Port: backupPort,
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
