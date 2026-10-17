package main

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

type LogEntry struct {
	Level   LogLevel
	Date    time.Time
	Message string
}

func invalidLine(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidLogLine, fmt.Sprintf(format, args...))
}

func ParseLogLine(line string) (LogEntry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return LogEntry{}, invalidLine("line is empty")
	}
	if line[0] != '[' {
		return LogEntry{}, invalidLine("must start with a [LEVEL] tag")
	}

	end := strings.IndexByte(line, ']')
	if end == -1 {
		return LogEntry{}, invalidLine("missing closing ']' after level")
	}

	level := LogLevel(line[1:end])
	switch level {
	case LevelInfo, LevelWarn, LevelError:
	default:
		return LogEntry{}, invalidLine("unknown level %q", string(level))
	}

	rest := strings.TrimSpace(line[end+1:])
	dateStr, message, found := strings.Cut(rest, " ")
	if !found {
		return LogEntry{}, invalidLine("missing message after date")
	}

	date, err := time.Parse(dateLayout, dateStr)
	if err != nil {
		return LogEntry{}, invalidLine("bad date %q (want YYYY-MM-DD)", dateStr)
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return LogEntry{}, invalidLine("message is empty")
	}

	return LogEntry{Level: level, Date: date, Message: message}, nil
}

func ParseBatch(logs []string) ([]LogEntry, error) {
	if len(logs) == 0 {
		return nil, ErrEmptyBatch
	}
	entries := make([]LogEntry, 0, len(logs))
	for i, line := range logs {
		entry, err := ParseLogLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
