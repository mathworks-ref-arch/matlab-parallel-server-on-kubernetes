// Package logging contains functions to create a logger
// Copyright 2024 The MathWorks, Inc.
package logging

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Wrapper around a zap logger that allows the log file to be closed
type Logger struct {
	*zap.Logger
	logFile *os.File
}

// Gracefully close a logger
func (l *Logger) Close() {
	l.Logger.Sync()
	if l.logFile != nil {
		l.logFile.Sync()
		l.logFile.Close()
	}
}

// Create a new logger. If logfile is empty, the logger writes to stdout; otherwise, it writes to a file.
func NewLogger(logfile string, logLevel int) (*Logger, error) {
	return createLogger(logfile, getZapLevel(logLevel))
}

// Create a logger wrapping an existing zap logger
func NewFromZapLogger(zl *zap.Logger) *Logger {
	return &Logger{zl, nil}
}

const (
	warnLevelThreshold  = 1
	infoLevelThreshold  = 2
	debugLevelThreshold = 5
)

// Convert a log level number to a zap logging level
func getZapLevel(logLevel int) zapcore.Level {
	if logLevel >= debugLevelThreshold {
		return zapcore.DebugLevel
	} else if logLevel >= infoLevelThreshold {
		return zapcore.InfoLevel
	} else if logLevel >= warnLevelThreshold {
		return zapcore.WarnLevel
	}
	return zapcore.ErrorLevel
}

const timeFormat = "2006 01 02 15:04:05.000 MST"

func createLogger(logFile string, level zapcore.Level) (*Logger, error) {
	var file *os.File
	var err error
	useLogFile := logFile != ""
	if useLogFile {
		file, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	if err != nil {
		return nil, err
	}

	timeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format(timeFormat))
	}

	encoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		EncodeTime:       timeEncoder,
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		ConsoleSeparator: " | ",
		TimeKey:          "ts",
		LevelKey:         "level",
		MessageKey:       "msg",
	})
	var core zapcore.Core
	if useLogFile {
		core = zapcore.NewCore(encoder, zapcore.AddSync(file), level)
	} else {
		core = zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	}

	return &Logger{
		Logger:  zap.New(core),
		logFile: file,
	}, nil
}
