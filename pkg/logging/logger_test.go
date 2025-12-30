package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"DEBUG", DEBUG},
		{"debug", DEBUG},
		{"INFO", INFO},
		{"info", INFO},
		{"WARN", WARN},
		{"warn", WARN},
		{"WARNING", WARN},
		{"warning", WARN},
		{"ERROR", ERROR},
		{"error", ERROR},
		{"unknown", INFO}, // default
		{"", INFO},        // default
	}

	for _, tt := range tests {
		if got := ParseLevel(tt.input); got != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestJSONLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(DEBUG, &buf)

	logger.Info("test message", F("key", "value"))

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("Level = %q, want %q", entry.Level, "INFO")
	}
	if entry.Message != "test message" {
		t.Errorf("Message = %q, want %q", entry.Message, "test message")
	}
	if entry.Fields["key"] != "value" {
		t.Errorf("Fields[key] = %v, want %q", entry.Fields["key"], "value")
	}
	if entry.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestJSONLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(INFO, &buf)

	// DEBUG should be filtered out
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("DEBUG message should be filtered when level is INFO")
	}

	// INFO should be output
	logger.Info("info message")
	if buf.Len() == 0 {
		t.Error("INFO message should be output when level is INFO")
	}

	buf.Reset()

	// WARN should be output
	logger.Warn("warn message")
	if buf.Len() == 0 {
		t.Error("WARN message should be output when level is INFO")
	}

	buf.Reset()

	// ERROR should be output
	logger.Error("error message")
	if buf.Len() == 0 {
		t.Error("ERROR message should be output when level is INFO")
	}
}

func TestJSONLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(DEBUG, &buf)

	// Create logger with base fields
	childLogger := logger.WithFields(F("component", "test"), F("version", "1.0"))

	childLogger.Info("test message", F("extra", "field"))

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Fields["component"] != "test" {
		t.Errorf("Fields[component] = %v, want %q", entry.Fields["component"], "test")
	}
	if entry.Fields["version"] != "1.0" {
		t.Errorf("Fields[version] = %v, want %q", entry.Fields["version"], "1.0")
	}
	if entry.Fields["extra"] != "field" {
		t.Errorf("Fields[extra] = %v, want %q", entry.Fields["extra"], "field")
	}
}

func TestJSONLoggerAllLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(DEBUG, &buf)

	tests := []struct {
		logFunc func(string, ...Field)
		level   string
	}{
		{logger.Debug, "DEBUG"},
		{logger.Info, "INFO"},
		{logger.Warn, "WARN"},
		{logger.Error, "ERROR"},
	}

	for _, tt := range tests {
		buf.Reset()
		tt.logFunc("test message")

		var entry LogEntry
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal log entry for %s: %v", tt.level, err)
		}

		if entry.Level != tt.level {
			t.Errorf("Level = %q, want %q", entry.Level, tt.level)
		}
	}
}

func TestJSONLoggerTimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(DEBUG, &buf)

	logger.Info("test")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	// Check RFC3339 format (e.g., 2006-01-02T15:04:05Z)
	if !strings.Contains(entry.Timestamp, "T") || !strings.HasSuffix(entry.Timestamp, "Z") {
		t.Errorf("Timestamp %q is not in RFC3339 format", entry.Timestamp)
	}
}

func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()

	// These should not panic
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")

	child := logger.WithFields(F("key", "value"))
	if child != logger {
		t.Error("NopLogger.WithFields should return itself")
	}
}

func TestNewJSONLoggerFromString(t *testing.T) {
	logger := NewJSONLoggerFromString("WARN", nil)
	if logger.GetLevel() != WARN {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), WARN)
	}
}

func TestJSONLoggerSetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(ERROR, &buf)

	// INFO should be filtered
	logger.Info("test")
	if buf.Len() > 0 {
		t.Error("INFO should be filtered when level is ERROR")
	}

	// Change level to DEBUG
	logger.SetLevel(DEBUG)

	// Now INFO should be output
	logger.Info("test")
	if buf.Len() == 0 {
		t.Error("INFO should be output after SetLevel(DEBUG)")
	}
}

// ============================================================================
// Property-Based Tests
// ============================================================================
// Feature: robustness-improvements, Property 5: JSON Log Format Consistency
// Feature: robustness-improvements, Property 6: Log Level Filtering
// Validates: Requirements 4.1, 4.2, 4.5

// Property 5: JSON Log Format Consistency
// *For any* log message, the output SHALL be valid JSON containing at minimum:
// timestamp (ISO8601), level (DEBUG|INFO|WARN|ERROR), and message fields.
// Validates: Requirements 4.1, 4.2
func TestProperty_JSONLogFormatConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate random log messages
	properties.Property("All log outputs are valid JSON with required fields", prop.ForAll(
		func(msg string, levelInt int) bool {
			var buf bytes.Buffer
			logger := NewJSONLogger(DEBUG, &buf)

			// Map int to level (0-3)
			level := Level(levelInt % 4)

			// Log at the specified level
			switch level {
			case DEBUG:
				logger.Debug(msg)
			case INFO:
				logger.Info(msg)
			case WARN:
				logger.Warn(msg)
			case ERROR:
				logger.Error(msg)
			}

			// Parse the output as JSON
			var entry LogEntry
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				return false
			}

			// Verify required fields exist
			if entry.Timestamp == "" {
				return false
			}
			if entry.Level == "" {
				return false
			}
			if entry.Message != msg {
				return false
			}

			// Verify level is one of the valid values
			validLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
			if !validLevels[entry.Level] {
				return false
			}

			// Verify timestamp is in ISO8601/RFC3339 format (contains T and ends with Z)
			if !strings.Contains(entry.Timestamp, "T") || !strings.HasSuffix(entry.Timestamp, "Z") {
				return false
			}

			return true
		},
		gen.AnyString(),
		gen.IntRange(0, 3),
	))

	// Test with fields
	properties.Property("Log outputs with fields are valid JSON", prop.ForAll(
		func(msg string, fieldKey string, fieldValue string) bool {
			var buf bytes.Buffer
			logger := NewJSONLogger(DEBUG, &buf)

			// Skip empty field keys as they're not valid
			if fieldKey == "" {
				return true
			}

			logger.Info(msg, F(fieldKey, fieldValue))

			var entry LogEntry
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				return false
			}

			// Verify the field is present
			if entry.Fields == nil {
				return false
			}
			if entry.Fields[fieldKey] != fieldValue {
				return false
			}

			return true
		},
		gen.AnyString(),
		gen.AlphaString(),
		gen.AnyString(),
	))

	properties.TestingRun(t)
}

// Property 6: Log Level Filtering
// *For any* log message with level L and configured minimum level M,
// the message SHALL be output if and only if L >= M
// (where DEBUG < INFO < WARN < ERROR).
// Validates: Requirements 4.5
func TestProperty_LogLevelFiltering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Messages are filtered correctly based on log level", prop.ForAll(
		func(msg string, configuredLevelInt int, messageLevelInt int) bool {
			var buf bytes.Buffer

			// Map ints to levels (0-3)
			configuredLevel := Level(configuredLevelInt % 4)
			messageLevel := Level(messageLevelInt % 4)

			logger := NewJSONLogger(configuredLevel, &buf)

			// Log at the message level
			switch messageLevel {
			case DEBUG:
				logger.Debug(msg)
			case INFO:
				logger.Info(msg)
			case WARN:
				logger.Warn(msg)
			case ERROR:
				logger.Error(msg)
			}

			// Check if output was produced
			hasOutput := buf.Len() > 0

			// Expected: output if messageLevel >= configuredLevel
			shouldOutput := messageLevel >= configuredLevel

			return hasOutput == shouldOutput
		},
		gen.AnyString(),
		gen.IntRange(0, 3),
		gen.IntRange(0, 3),
	))

	// Test specific level ordering: DEBUG < INFO < WARN < ERROR
	properties.Property("Level ordering is correct: DEBUG < INFO < WARN < ERROR", prop.ForAll(
		func(dummy int) bool {
			return DEBUG < INFO && INFO < WARN && WARN < ERROR
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}
