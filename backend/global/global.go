package global

import (
	"sagiri-guard/backend/config"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

var (
	Config config.Config
	Logger zerolog.Logger
	Mdb    *gorm.DB
	Rdb    *redis.Client
)
