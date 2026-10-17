package main

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrEmptyTaskID       = errors.New("task id must not be empty")
	ErrNilTask           = errors.New("task must not be nil")
	ErrNilRepository     = errors.New("repository must not be nil")
	ErrEmptyBatch        = errors.New("log batch must contain at least one line")
	ErrInvalidLogLine    = errors.New("invalid log line")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskExists        = errors.New("task already exists")
)

type TaskState string

const (
	StatePending    TaskState = "PENDING"
	StateProcessing TaskState = "PROCESSING"
	StateCompleted  TaskState = "COMPLETED"
	StateFailed     TaskState = "FAILED"
)

var allowedTransitions = map[TaskState][]TaskState{
	StatePending:    {StateProcessing, StateFailed},
	StateProcessing: {StateCompleted, StateFailed},
	StateCompleted:  nil,
	StateFailed:     nil,
}

func (s TaskState) CanTransitionTo(next TaskState) bool {
	return slices.Contains(allowedTransitions[s], next)
}

type LogLevel string

const (
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

var AllLevels = []LogLevel{LevelInfo, LevelWarn, LevelError}

type Alert struct {
	LineNumber int
	Level      LogLevel
	Trigger    string
	Message    string
}

type Result struct {
	TotalLines int
	Stats      map[LogLevel]int
	Alerts     []Alert
}

func (r *Result) Clone() *Result {
	if r == nil {
		return nil
	}
	c := *r
	c.Stats = make(map[LogLevel]int, len(r.Stats))
	for level, n := range r.Stats {
		c.Stats[level] = n
	}
	c.Alerts = slices.Clone(r.Alerts)
	return &c
}

type Task struct {
	ID            string
	Logs          []string
	State         TaskState
	Result        *Result
	FailureReason string
}

func NewTask(id string, logs []string) (*Task, error) {
	if id == "" {
		return nil, ErrEmptyTaskID
	}
	return &Task{
		ID:    id,
		Logs:  slices.Clone(logs),
		State: StatePending,
	}, nil
}

func (t *Task) TransitionTo(next TaskState) error {
	if !t.State.CanTransitionTo(next) {
		return fmt.Errorf("task %q: %s -> %s: %w", t.ID, t.State, next, ErrInvalidTransition)
	}
	t.State = next
	return nil
}

func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	c := *t
	c.Logs = slices.Clone(t.Logs)
	c.Result = t.Result.Clone()
	return &c
}
