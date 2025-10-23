package logger

import (
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var L zerolog.Logger

func Init(path string) error {
	var w io.Writer = os.Stdout
	if path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		w = file
	}
	L = log.Output(zerolog.ConsoleWriter{Out: w})
	return nil
}

func Info(v ...interface{})                     { L.Info().Msgf("%v", v...) }
func Error(v ...interface{})                    { L.Error().Msgf("%v", v...) }
func Infof(f string, v ...interface{})          { L.Info().Msgf(f, v...) }
func Errorf(f string, v ...interface{})         { L.Error().Msgf(f, v...) }
func Sprintf(f string, v ...interface{}) string { return fmt.Sprintf(f, v...) }
