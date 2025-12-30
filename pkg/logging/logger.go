// Package logging 提供结构化日志功能
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	// DEBUG 调试级别
	DEBUG Level = iota
	// INFO 信息级别
	INFO
	// WARN 警告级别
	WARN
	// ERROR 错误级别
	ERROR
)

// String 返回日志级别的字符串表示
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel 从字符串解析日志级别
func ParseLevel(s string) Level {
	switch s {
	case "DEBUG", "debug":
		return DEBUG
	case "INFO", "info":
		return INFO
	case "WARN", "warn", "WARNING", "warning":
		return WARN
	case "ERROR", "error":
		return ERROR
	default:
		return INFO
	}
}

// Field 日志字段
type Field struct {
	Key   string
	Value interface{}
}

// F 创建日志字段的便捷函数
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Logger 结构化日志接口
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	WithFields(fields ...Field) Logger
}

// LogEntry JSON 日志条目
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// JSONLogger JSON 格式日志实现
type JSONLogger struct {
	level      Level
	output     io.Writer
	mu         sync.Mutex
	baseFields map[string]interface{}
}

// NewJSONLogger 创建 JSON 日志器
func NewJSONLogger(level Level, output io.Writer) *JSONLogger {
	if output == nil {
		output = os.Stdout
	}
	return &JSONLogger{
		level:      level,
		output:     output,
		baseFields: make(map[string]interface{}),
	}
}

// NewJSONLoggerFromString 从字符串级别创建 JSON 日志器
func NewJSONLoggerFromString(levelStr string, output io.Writer) *JSONLogger {
	return NewJSONLogger(ParseLevel(levelStr), output)
}

// shouldLog 判断是否应该输出日志
func (l *JSONLogger) shouldLog(level Level) bool {
	return level >= l.level
}

// log 内部日志方法
func (l *JSONLogger) log(level Level, msg string, fields ...Field) {
	if !l.shouldLog(level) {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level.String(),
		Message:   msg,
	}

	// 合并基础字段和传入字段
	if len(l.baseFields) > 0 || len(fields) > 0 {
		entry.Fields = make(map[string]interface{})
		for k, v := range l.baseFields {
			entry.Fields[k] = v
		}
		for _, f := range fields {
			entry.Fields[f.Key] = f.Value
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// 回退到 stderr
		fmt.Fprintf(os.Stderr, "failed to marshal log entry: %v\n", err)
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, err = l.output.Write(append(data, '\n'))
	if err != nil {
		// 回退到 stderr
		fmt.Fprintf(os.Stderr, "failed to write log: %v\n", err)
	}
}

// Debug 输出调试日志
func (l *JSONLogger) Debug(msg string, fields ...Field) {
	l.log(DEBUG, msg, fields...)
}

// Info 输出信息日志
func (l *JSONLogger) Info(msg string, fields ...Field) {
	l.log(INFO, msg, fields...)
}

// Warn 输出警告日志
func (l *JSONLogger) Warn(msg string, fields ...Field) {
	l.log(WARN, msg, fields...)
}

// Error 输出错误日志
func (l *JSONLogger) Error(msg string, fields ...Field) {
	l.log(ERROR, msg, fields...)
}

// WithFields 返回带有预设字段的新 Logger
func (l *JSONLogger) WithFields(fields ...Field) Logger {
	newLogger := &JSONLogger{
		level:      l.level,
		output:     l.output,
		baseFields: make(map[string]interface{}),
	}

	// 复制现有基础字段
	for k, v := range l.baseFields {
		newLogger.baseFields[k] = v
	}

	// 添加新字段
	for _, f := range fields {
		newLogger.baseFields[f.Key] = f.Value
	}

	return newLogger
}

// SetLevel 设置日志级别
func (l *JSONLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel 获取当前日志级别
func (l *JSONLogger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// NopLogger 空日志器，不输出任何内容
type NopLogger struct{}

// NewNopLogger 创建空日志器
func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

// Debug 空实现
func (l *NopLogger) Debug(msg string, fields ...Field) {}

// Info 空实现
func (l *NopLogger) Info(msg string, fields ...Field) {}

// Warn 空实现
func (l *NopLogger) Warn(msg string, fields ...Field) {}

// Error 空实现
func (l *NopLogger) Error(msg string, fields ...Field) {}

// WithFields 返回自身
func (l *NopLogger) WithFields(fields ...Field) Logger {
	return l
}
