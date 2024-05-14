package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// Test logging to an output file
func TestLoggerWithOutfile(t *testing.T) {
	testCases := []struct {
		name             string
		logLevel         int
		expectedZapLevel zapcore.Level
	}{
		{"level0", 0, zapcore.ErrorLevel},
		{"level1", 1, zapcore.WarnLevel},
		{"level2", 2, zapcore.InfoLevel},
		{"level3", 3, zapcore.InfoLevel},
		{"level4", 4, zapcore.InfoLevel},
		{"level5", 5, zapcore.DebugLevel},
		{"level6", 6, zapcore.DebugLevel},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			verifyLogFile(tt, tc.logLevel, tc.expectedZapLevel)
		})
	}
}

// Test that we can log to stdout without errors
func TestLoggerStdout(t *testing.T) {
	logCloser, err := NewLogger("", 1)
	require.NoError(t, err, "Error creating logger without a log file")
	require.NotNil(t, logCloser.Logger, "Zap logger should not be nil")
	require.Nil(t, logCloser.logFile, "Log file should be nil for logger without an output file")
	logCloser.Logger.Info("test message")
	logCloser.Close()
}

// Check we get an error when we cannot open the log file
func TestLoggerError(t *testing.T) {
	badFilePath := "/this/does/not/exist.log"
	_, err := NewLogger(badFilePath, 1)
	assert.Error(t, err, "Should get an error when attempting to create logger with invalid file path")
}

func verifyLogFile(t *testing.T, logLevel int, expectedZapLevel zapcore.Level) {
	outdir := t.TempDir()
	outfile := filepath.Join(outdir, "test.log")
	var logger *Logger
	var err error
	logger, err = NewLogger(outfile, logLevel)
	require.NoError(t, err, "Error creating logger")
	require.NotNil(t, logger.logFile, "Log file should not be nil")
	require.NotNil(t, logger.Logger, "Zap logger should not be nil")

	// Write some messages
	logMessages := map[zapcore.Level]string{
		zapcore.DebugLevel: "this is a debug message",
		zapcore.InfoLevel:  "this is an info message",
		zapcore.WarnLevel:  "this is a warning message",
		zapcore.ErrorLevel: "this is an error message",
	}
	logger.Debug(logMessages[zapcore.DebugLevel])
	logger.Info(logMessages[zapcore.InfoLevel])
	logger.Warn(logMessages[zapcore.WarnLevel])
	logger.Error(logMessages[zapcore.ErrorLevel])

	// Close the logger
	logger.Close()

	// Check the file contents
	fileBytes, err := os.ReadFile(outfile)
	fileContent := string(fileBytes)
	require.NoError(t, err, "Error reading log file")
	verifyTimestamps(t, fileContent)

	// Check the correct level of messages are logged
	for zapLevel, msg := range logMessages {
		zapLevelStr := zapLevel.CapitalString()
		if zapLevel >= expectedZapLevel {
			verifyLogMessage(t, fileContent, msg, zapLevelStr)
		} else {
			assert.NotContainsf(t, fileContent, msg, "Log file should not contain message for level %s", zapLevelStr)
		}
	}
}

// Check that the log file contents contain a given message
func verifyLogMessage(t *testing.T, fileContent, expectedMsg, level string) {
	msgWithLevel := fmt.Sprintf("%s | %s", level, expectedMsg)
	assert.Contains(t, fileContent, msgWithLevel, "Log file should contain log level and message")
}

// Check that the log file contents contain the expected timestamps
func verifyTimestamps(t *testing.T, fileContent string) {
	lines := strings.Split(fileContent, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		sections := strings.Split(line, " | ")
		timestamp := sections[0]

		// Check the timestamp is in UTC
		assert.Contains(t, timestamp, "UTC", "Timestamp should be in UTC")

		// Check we can convert the timestamp back into the expected time
		_, err := time.Parse(timeFormat, timestamp)
		assert.NoErrorf(t, err, "Should be able to convert timestamp \"%\" using the time format \"%s\"", timestamp, timeFormat)
	}
}
