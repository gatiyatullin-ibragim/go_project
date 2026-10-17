package main

import (
	"errors"
	"fmt"
	"slices"
)

// Sentinel errors. Callers are expected to match them with errors.Is, so every
// place that returns one of them wraps it with %w and adds context.
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

// TaskState is the position of a task in its lifecycle.
type TaskState string

const (
	StatePending    TaskState = "PENDING"
	StateProcessing TaskState = "PROCESSING"
	StateCompleted  TaskState = "COMPLETED"
	StateFailed     TaskState = "FAILED"
)

// allowedTransitions is the single source of truth for the state machine.
// Completed and Failed are terminal: they have no outgoing edges, which is
// what blocks re-processing a finished task.
//
// Pending -> Failed exists so that a batch rejected by validation can be
// recorded as failed without ever having been "processing".
var allowedTransitions = map[TaskState][]TaskState{
	StatePending:    {StateProcessing, StateFailed},
	StateProcessing: {StateCompleted, StateFailed},
	StateCompleted:  nil,
	StateFailed:     nil,
}

// CanTransitionTo reports whether moving from s to next is permitted.
// Unknown states have no entry in the map, so they can never transition.
func (s TaskState) CanTransitionTo(next TaskState) bool {
	return slices.Contains(allowedTransitions[s], next)
}

// LogLevel is the severity extracted from a log line.
type LogLevel string

const (
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// AllLevels lists the supported levels in a stable display order
// (map iteration order is random, so reports iterate over this instead).
var AllLevels = []LogLevel{LevelInfo, LevelWarn, LevelError}

// Alert is raised when a log message contains a trigger word.
type Alert struct {
	LineNumber int      // 1-based position of the line in the batch
	Level      LogLevel // level of the offending line
	Trigger    string   // the trigger word that matched
	Message    string   // the message part of the line (without level/date)
}

// Result holds everything the processor derives from a batch.
type Result struct {
	TotalLines int
	Stats      map[LogLevel]int
	Alerts     []Alert
}

// Clone returns a deep copy of the result. Maps and slices are reference
// types, so a plain struct copy would still share their backing storage.
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

// Task is one unit of work: a batch of raw log lines plus its lifecycle data.
//
// Design note (pointer vs value): Task is passed around as *Task because it is
// a mutable entity with identity - the processor changes its State and Result,
// and callers expect to observe those changes on the task they hold. Result is
// a *Result (not a value) so that "no result yet" is representable as nil, and
// because it is only ever created once, when processing completes.
type Task struct {
	ID            string
	Logs          []string
	State         TaskState
	Result        *Result // nil until the task is Completed
	FailureReason string  // set when the task is Failed
}

// NewTask builds a Pending task. The input slice is copied so later changes by
// the caller cannot silently alter the task's payload.
//
// Input content is deliberately NOT validated here: an invalid batch is a
// legitimate task that the processor must be able to reject and mark Failed.
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

// TransitionTo moves the task to next if the state machine allows it.
// On failure the task is left untouched.
func (t *Task) TransitionTo(next TaskState) error {
	if !t.State.CanTransitionTo(next) {
		return fmt.Errorf("task %q: %s -> %s: %w", t.ID, t.State, next, ErrInvalidTransition)
	}
	t.State = next
	return nil
}

// Clone returns a deep copy of the task. The repository uses it to guarantee
// that stored data and caller-held data never alias each other.
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	c := *t
	c.Logs = slices.Clone(t.Logs)
	c.Result = t.Result.Clone()
	return &c
}
