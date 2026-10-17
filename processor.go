package main

import (
	"fmt"
	"strings"
)

// DefaultTriggerWords are the (case-insensitive) words that raise an alert.
var DefaultTriggerWords = []string{"timeout", "failed", "panic"}

// Processor validates tasks, runs the log analysis and persists the outcome.
//
// Design note: it holds a TaskRepository (interface), not *MemoryTaskRepository,
// and is itself used through a pointer so one configured instance is shared
// rather than copied.
type Processor struct {
	repo     TaskRepository
	triggers []string // lower-case, non-empty
}

// NewProcessor creates a processor. If no trigger words are given,
// DefaultTriggerWords are used.
func NewProcessor(repo TaskRepository, triggerWords ...string) (*Processor, error) {
	if repo == nil {
		return nil, ErrNilRepository
	}

	var triggers []string
	for _, w := range triggerWords {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			triggers = append(triggers, w)
		}
	}
	if len(triggers) == 0 {
		triggers = append(triggers, DefaultTriggerWords...)
	}
	return &Processor{repo: repo, triggers: triggers}, nil
}

// Submit creates a Pending task and stores it.
func (p *Processor) Submit(id string, logs []string) (*Task, error) {
	task, err := NewTask(id, logs)
	if err != nil {
		return nil, err
	}
	if err := p.repo.Create(task); err != nil {
		return nil, err
	}
	return task, nil
}

// ProcessByID loads a stored task, processes it and returns the updated task.
func (p *Processor) ProcessByID(id string) (*Task, error) {
	task, err := p.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := p.Process(task); err != nil {
		return task, err
	}
	return task, nil
}

// Process runs a Pending task through its lifecycle:
//
//	Pending -> Processing -> Completed
//	Pending -> Processing -> Failed     (invalid input)
//
// Errors:
//   - ErrInvalidTransition: the task is not Pending (e.g. already Completed).
//   - ErrEmptyBatch / ErrInvalidLogLine: the input was rejected; the task is
//     stored as Failed with FailureReason set.
//   - ErrTaskNotFound: the task was never created in the repository.
//
// The work is done on a private copy, and the caller's task is updated only
// after the repository accepted the new state. If persistence fails, the
// caller's task is left exactly as it was instead of drifting out of sync
// with storage.
func (p *Processor) Process(task *Task) error {
	if task == nil {
		return ErrNilTask
	}

	work := task.Clone()

	// Guard: also what blocks Completed/Failed (terminal) and Processing tasks.
	if err := work.TransitionTo(StateProcessing); err != nil {
		return err
	}

	// Validate before any business logic runs.
	entries, err := ParseBatch(work.Logs)
	if err != nil {
		return p.reject(task, work, err)
	}

	if err := p.repo.Save(work); err != nil {
		return fmt.Errorf("persist processing state: %w", err)
	}

	work.Result = p.analyze(entries)
	if err := work.TransitionTo(StateCompleted); err != nil {
		return err
	}
	if err := p.repo.Save(work); err != nil {
		return fmt.Errorf("persist result: %w", err)
	}

	*task = *work
	return nil
}

// reject records a validation failure: Processing -> Failed, then persist.
func (p *Processor) reject(task, work *Task, cause error) error {
	if err := work.TransitionTo(StateFailed); err != nil {
		return err
	}
	work.FailureReason = cause.Error()
	if err := p.repo.Save(work); err != nil {
		return fmt.Errorf("persist failed state: %w", err)
	}
	*task = *work
	return fmt.Errorf("task %q rejected: %w", task.ID, cause)
}

// analyze is the pure business logic: it counts levels and raises alerts.
// A line produces one alert per trigger word it contains. Matching is a
// case-insensitive substring search on the message only, so the level tag
// itself (e.g. "ERROR") can never trigger an alert.
func (p *Processor) analyze(entries []LogEntry) *Result {
	res := &Result{
		TotalLines: len(entries),
		Stats:      make(map[LogLevel]int, len(AllLevels)),
	}
	for _, level := range AllLevels {
		res.Stats[level] = 0 // report zeros explicitly instead of omitting keys
	}

	for i, e := range entries {
		res.Stats[e.Level]++

		lowered := strings.ToLower(e.Message)
		for _, trigger := range p.triggers {
			if strings.Contains(lowered, trigger) {
				res.Alerts = append(res.Alerts, Alert{
					LineNumber: i + 1,
					Level:      e.Level,
					Trigger:    trigger,
					Message:    e.Message,
				})
			}
		}
	}
	return res
}
