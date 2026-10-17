package main

import (
	"fmt"
	"strings"
)

var DefaultTriggerWords = []string{"timeout", "failed", "panic"}

type Processor struct {
	repo     TaskRepository
	triggers []string
}

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

func (p *Processor) Process(task *Task) error {
	if task == nil {
		return ErrNilTask
	}

	work := task.Clone()

	if err := work.TransitionTo(StateProcessing); err != nil {
		return err
	}

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

func (p *Processor) analyze(entries []LogEntry) *Result {
	res := &Result{
		TotalLines: len(entries),
		Stats:      make(map[LogLevel]int, len(AllLevels)),
	}
	for _, level := range AllLevels {
		res.Stats[level] = 0
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
