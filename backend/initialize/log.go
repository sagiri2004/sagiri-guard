package initialize

import (
	"os"
	"sagiri-guard/backend/global"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	// basic zerolog setup: console writer to stdout
	cw := zerolog.ConsoleWriter{Out: os.Stdout}
	logger := log.Output(cw)
	global.Logger = logger
}
