package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
)

// LogLevel provides compariable levels and a string representation.
type LogLevel int

const (
	_ LogLevel = iota
	TRACE
	DEBUG
	INFO
	ERROR
	DEADLETTER
)

// String formats LogLevels as a string for readability.
func (l LogLevel) String() string {
	var values = make(map[LogLevel]string)
	values[TRACE] = "trace"
	values[DEBUG] = "debug"
	values[INFO] = "info"
	values[ERROR] = "error"
	values[DEADLETTER] = "deadletter"

	s, ok := values[l]
	if !ok {
		return "unknown"
	}
	return s
}

// Logger acts as a single config point for emitting FollowerLogs as JSON.
type Logger struct {
	verbosity LogLevel
	logger    *slog.Logger
}

// NewLogger creates a new Logger with the specified verbosity level.
func NewLogger(verbosity LogLevel) Logger {
	// Create a JSON handler for slog
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevelFromLogLevel(verbosity),
	})
	return Logger{
		verbosity: verbosity,
		logger:    slog.New(handler),
	}
}

// slogLevelFromLogLevel converts our custom LogLevel to slog.Level
func slogLevelFromLogLevel(level LogLevel) slog.Level {
	switch level {
	case TRACE:
		return slog.LevelDebug - 1 // Trace is lower than Debug
	case DEBUG:
		return slog.LevelDebug
	case INFO:
		return slog.LevelInfo
	case ERROR:
		return slog.LevelError
	case DEADLETTER:
		return slog.LevelError + 1 // DeadLetter is higher than Error
	default:
		return slog.LevelInfo
	}
}

func (l Logger) logAtLevel(name string, level LogLevel, message string) {
	if level >= l.verbosity {
		slogLevel := slogLevelFromLogLevel(level)
		l.logger.Log(context.Background(), slogLevel, message, slog.String("name", name))
	}
}

func (l Logger) logFormatAtLevel(name string, level LogLevel, message string, f ...interface{}) {
	msg := fmt.Sprintf(message, f...)
	l.logAtLevel(name, level, msg)
}

func (l Logger) Trace(name, message string) {
	l.logAtLevel(name, TRACE, message)
}

func (l Logger) Tracef(name, message string, f ...interface{}) {
	l.logFormatAtLevel(name, TRACE, message, f...)
}

func (l Logger) Debug(name, message string) {
	l.logAtLevel(name, DEBUG, message)
}

func (l Logger) Debugf(name, message string, f ...interface{}) {
	l.logFormatAtLevel(name, DEBUG, message, f...)
}

func (l Logger) Info(name, message string) {
	l.logAtLevel(name, INFO, message)
}

func (l Logger) Infof(name, message string, f ...interface{}) {
	l.logFormatAtLevel(name, INFO, message, f...)
}

func (l Logger) Error(name, message string) {
	l.logAtLevel(name, ERROR, message)
}

func (l Logger) Errorf(name, message string, f ...interface{}) {
	l.logFormatAtLevel(name, ERROR, message, f...)
}

func (l Logger) DeadLetter(name string, rawLog NomadLog, message string) {
	data := make(map[string]interface{})
	_ = mapstructure.Decode(rawLog, &data)

	// Convert data map to slog attributes
	attrs := []slog.Attr{
		slog.String("name", name),
		slog.String("log_level", DEADLETTER.String()),
		slog.Time("datetime", time.Now()),
	}
	for k, v := range data {
		attrs = append(attrs, slog.Any(k, v))
	}

	l.logger.LogAttrs(context.Background(), slogLevelFromLogLevel(DEADLETTER), message, attrs...)
}

func (l Logger) DeadLetterf(name string, rawLog NomadLog, message string, f ...interface{}) {
	msg := fmt.Sprintf(message, f...)
	l.DeadLetter(name, rawLog, msg)
}
