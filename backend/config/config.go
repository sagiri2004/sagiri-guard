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

type Config struct {
	HTTP HTTP
	TCP  TCP
	DB   DB
	JWT  struct {
		Secret string
		Issuer string
		ExpMin int
	}
}

func Load(path string) (Config, error) {
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

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := Config{
		HTTP: HTTP{Host: v.GetString("backend.http.host"), Port: v.GetInt("backend.http.port")},
		TCP:  TCP{Host: v.GetString("backend.tcp.host"), Port: v.GetInt("backend.tcp.port")},
		DB:   DB{Host: v.GetString("backend.db.host"), Port: v.GetInt("backend.db.port"), User: v.GetString("backend.db.user"), Pass: v.GetString("backend.db.pass"), Name: v.GetString("backend.db.name")},
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
	if cfg.JWT.ExpMin == 0 {
		cfg.JWT.ExpMin = 60
	}
	return cfg, nil
}
