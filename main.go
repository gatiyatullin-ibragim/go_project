package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run holds the demo so errors can be returned instead of handled inline
// (and os.Exit is only called in one place).
func run() error {
	repo := NewMemoryTaskRepository()
	proc, err := NewProcessor(repo)
	if err != nil {
		return err
	}

	// 1. Happy path.
	if _, err := proc.Submit("batch-1", []string{
		"[INFO] 2026-10-15 Service started",
		"[ERROR] 2026-10-15 DB connection timeout",
		"[WARN] 2026-10-15 Disk usage high",
		"[ERROR] 2026-10-15 Request failed after retry",
		"[INFO] 2026-10-16 Worker panic recovered",
	}); err != nil {
		return err
	}
	task, err := proc.ProcessByID("batch-1")
	if err != nil {
		return err
	}
	printTask(task)

	// 2. Invalid input: the task ends up Failed and the error is typed.
	if _, err := proc.Submit("batch-2", []string{"[INFO] 2026-10-15 ok", "garbage line"}); err != nil {
		return err
	}
	task, err = proc.ProcessByID("batch-2")
	fmt.Printf("\nbatch-2 error: %v\n", err)
	fmt.Printf("  is ErrInvalidLogLine: %t\n", errors.Is(err, ErrInvalidLogLine))
	printTask(task)

	// 3. Strict state machine: a Completed task cannot be processed again.
	_, err = proc.ProcessByID("batch-1")
	fmt.Printf("\nre-processing batch-1: %v\n", err)
	fmt.Printf("  is ErrInvalidTransition: %t\n", errors.Is(err, ErrInvalidTransition))

	// 4. Unknown task.
	_, err = repo.GetByID("does-not-exist")
	fmt.Printf("\nlookup of missing task: %v\n", err)
	fmt.Printf("  is ErrTaskNotFound: %t\n", errors.Is(err, ErrTaskNotFound))

	return nil
}

func printTask(t *Task) {
	fmt.Printf("task %s [%s]\n", t.ID, t.State)
	if t.FailureReason != "" {
		fmt.Printf("  failure: %s\n", t.FailureReason)
	}
	if t.Result == nil {
		return
	}
	fmt.Printf("  lines: %d\n", t.Result.TotalLines)
	for _, level := range AllLevels {
		fmt.Printf("  %-5s %d\n", level, t.Result.Stats[level])
	}
	for _, a := range t.Result.Alerts {
		fmt.Printf("  ALERT line %d [%s] trigger=%q: %s\n", a.LineNumber, a.Level, a.Trigger, a.Message)
	}
}
