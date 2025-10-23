package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var L zerolog.Logger

func Init(path string) error {
	// console writer setup (human-friendly)
	cw := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}

	var writer io.Writer = cw
	if path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		fw := zerolog.ConsoleWriter{Out: file, NoColor: true, TimeFormat: "2006-01-02 15:04:05"}
		writer = zerolog.MultiLevelWriter(cw, fw)
	}
	zerolog.TimeFieldFormat = time.DateTime
	L = zerolog.New(writer).With().Timestamp().Logger()
	return nil
}

func Info(v ...interface{})                     { L.Info().Msgf("%v", v...) }
func Error(v ...interface{})                    { L.Error().Msgf("%v", v...) }
func Infof(f string, v ...interface{})          { L.Info().Msgf(f, v...) }
func Errorf(f string, v ...interface{})         { L.Error().Msgf(f, v...) }
func Sprintf(f string, v ...interface{}) string { return fmt.Sprintf(f, v...) }
