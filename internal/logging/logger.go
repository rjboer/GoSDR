package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
)

// Level represents a logging severity.
type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string to a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return Debug, nil
	case "info", "":
		return Info, nil
	case "warn", "warning":
		return Warn, nil
	case "error":
		return Error, nil
	default:
		return Level(0), fmt.Errorf("unsupported log level %q", s)
	}
}

// Format controls how log entries are rendered.
type Format int

const (
	Text Format = iota
	JSON
)

func (f Format) String() string {
	switch f {
	case Text:
		return "text"
	case JSON:
		return "json"
	default:
		return "unknown"
	}
}

// ParseFormat converts a string to a Format.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return JSON, nil
	case "text", "":
		return Text, nil
	default:
		return Format(0), fmt.Errorf("unsupported log format %q", s)
	}
}

// Field represents a structured log field.
type Field struct {
	Key   string
	Value any
}

// Logger defines leveled structured logging operations.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Default returns the process-wide logger.
func Default() Logger {
	if defaultLogger == nil {
		defaultLogger = New(Info, Text, io.Discard)
	}
	return defaultLogger
}

// SetDefault replaces the process-wide logger.
func SetDefault(l Logger) {
	if l != nil {
		defaultLogger = l
	}
}

var defaultLogger Logger

type baseLogger struct {
	level      Level
	format     Format
	fields     []Field
	underlying *log.Logger
}

// New constructs a Logger with the given level, format, and output writer.
func New(level Level, format Format, out io.Writer) Logger {
	return &baseLogger{
		level:      level,
		format:     format,
		underlying: log.New(out, "", log.LstdFlags),
	}
}

func (l *baseLogger) With(fields ...Field) Logger {
	combined := make([]Field, 0, len(l.fields)+len(fields))
	combined = append(combined, l.fields...)
	combined = append(combined, fields...)
	return &baseLogger{
		level:      l.level,
		format:     l.format,
		fields:     combined,
		underlying: l.underlying,
	}
}

func (l *baseLogger) Debug(msg string, fields ...Field) { l.log(Debug, msg, fields...) }
func (l *baseLogger) Info(msg string, fields ...Field)  { l.log(Info, msg, fields...) }
func (l *baseLogger) Warn(msg string, fields ...Field)  { l.log(Warn, msg, fields...) }
func (l *baseLogger) Error(msg string, fields ...Field) { l.log(Error, msg, fields...) }

func (l *baseLogger) log(level Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}
	allFields := append(append([]Field{}, l.fields...), fields...)
	switch l.format {
	case JSON:
		l.logJSON(level, msg, allFields)
	default:
		l.logText(level, msg, allFields)
	}
}

func (l *baseLogger) logText(level Level, msg string, fields []Field) {
	if len(fields) == 0 {
		l.underlying.Printf("[%s] %s", level.String(), msg)
		return
	}
	var b strings.Builder
	for i, f := range fields {
		if f.Key == "" {
			continue
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%v", f.Key, f.Value)
	}
	l.underlying.Printf("[%s] %s %s", level.String(), msg, b.String())
}

func (l *baseLogger) logJSON(level Level, msg string, fields []Field) {
	payload := map[string]any{
		"time":  time.Now().Format(time.RFC3339Nano),
		"level": level.String(),
		"msg":   msg,
	}
	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		payload[f.Key] = f.Value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		l.underlying.Printf("[ERROR] marshal log payload failed: %v", err)
		return
	}
	l.underlying.Print(string(data))
}
