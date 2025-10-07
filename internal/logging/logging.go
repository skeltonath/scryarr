package logging

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup initializes zerolog with the specified log level
func Setup(level string) {
	// Parse log level
	var zlevel zerolog.Level
	switch strings.ToLower(level) {
	case "debug":
		zlevel = zerolog.DebugLevel
	case "info":
		zlevel = zerolog.InfoLevel
	case "warn":
		zlevel = zerolog.WarnLevel
	case "error":
		zlevel = zerolog.ErrorLevel
	default:
		zlevel = zerolog.InfoLevel
	}

	// Use structured JSON logs for production
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zlevel)

	// For pretty console output in development
	var output io.Writer = os.Stdout
	if os.Getenv("ENV") == "development" {
		output = zerolog.ConsoleWriter{Out: os.Stdout}
	}

	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}

// GetLogger returns a logger with context fields
func GetLogger(component string) zerolog.Logger {
	return log.With().Str("component", component).Logger()
}
