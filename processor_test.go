package main

import (
	"errors"
	"reflect"
	"testing"
)

var sampleLogs = []string{
	"[INFO] 2026-10-15 Service started",
	"[ERROR] 2026-10-15 DB connection timeout",
	"[WARN] 2026-10-15 Disk usage high",
	"[ERROR] 2026-10-15 Request failed after retry",
	"[INFO] 2026-10-16 Worker PANIC recovered",
}

func newTestEnv(t *testing.T) (*MemoryTaskRepository, *Processor) {
	t.Helper()
	repo := NewMemoryTaskRepository()
	proc, err := NewProcessor(repo)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return repo, proc
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantLevel   LogLevel
		wantMessage string
		wantErr     bool
	}{
		{"valid error", "[ERROR] 2026-10-15 DB connection timeout", LevelError, "DB connection timeout", false},
		{"valid info", "[INFO] 2026-01-02 started", LevelInfo, "started", false},
		{"valid warn", "[WARN] 2026-01-02 disk high", LevelWarn, "disk high", false},
		{"surrounding whitespace trimmed", "  [INFO] 2026-01-02   hello  ", LevelInfo, "hello", false},
		{"empty line", "", "", "", true},
		{"whitespace only", "   \t ", "", "", true},
		{"no level tag", "ERROR 2026-10-15 something", "", "", true},
		{"missing closing bracket", "[INFO 2026-10-15 x", "", "", true},
		{"unknown level", "[DEBUG] 2026-10-15 x", "", "", true},
		{"lower-case level", "[error] 2026-10-15 x", "", "", true},
		{"empty level", "[] 2026-10-15 x", "", "", true},
		{"invalid date", "[INFO] 2026-13-45 x", "", "", true},
		{"wrong date format", "[INFO] 15/10/2026 x", "", "", true},
		{"missing message", "[INFO] 2026-10-15", "", "", true},
		{"blank message", "[INFO] 2026-10-15    ", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLogLine(tc.line)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidLogLine) {
					t.Fatalf("err = %v, want ErrInvalidLogLine", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Level != tc.wantLevel || got.Message != tc.wantMessage {
				t.Errorf("got (%s, %q), want (%s, %q)", got.Level, got.Message, tc.wantLevel, tc.wantMessage)
			}
		})
	}
}

func TestParseBatch(t *testing.T) {
	tests := []struct {
		name    string
		logs    []string
		wantLen int
		wantErr error
	}{
		{"valid batch", sampleLogs, 5, nil},
		{"nil batch", nil, 0, ErrEmptyBatch},
		{"empty batch", []string{}, 0, ErrEmptyBatch},
		{"one bad line rejects whole batch", []string{"[INFO] 2026-10-15 ok", "nope"}, 0, ErrInvalidLogLine},
		{"blank line inside batch", []string{"[INFO] 2026-10-15 ok", ""}, 0, ErrInvalidLogLine},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := ParseBatch(tc.logs)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if len(entries) != tc.wantLen {
				t.Errorf("len(entries) = %d, want %d", len(entries), tc.wantLen)
			}
		})
	}
}

func TestNewTask(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		task, err := NewTask("t1", sampleLogs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != "t1" || task.State != StatePending || task.Result != nil {
			t.Errorf("unexpected task: %+v", task)
		}
		if !reflect.DeepEqual(task.Logs, sampleLogs) {
			t.Errorf("logs not stored")
		}
	})

	t.Run("empty id", func(t *testing.T) {
		if _, err := NewTask("", sampleLogs); !errors.Is(err, ErrEmptyTaskID) {
			t.Errorf("err = %v, want ErrEmptyTaskID", err)
		}
	})

	t.Run("payload is copied", func(t *testing.T) {
		logs := []string{"[INFO] 2026-10-15 a"}
		task, _ := NewTask("t1", logs)
		logs[0] = "changed"
		if task.Logs[0] != "[INFO] 2026-10-15 a" {
			t.Errorf("task shares memory with caller's slice")
		}
	})
}

func TestNewProcessorNilRepo(t *testing.T) {
	if _, err := NewProcessor(nil); !errors.Is(err, ErrNilRepository) {
		t.Errorf("err = %v, want ErrNilRepository", err)
	}
}

func TestSubmitAndGet(t *testing.T) {
	repo, proc := newTestEnv(t)

	if _, err := proc.Submit("t1", sampleLogs); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got, err := repo.GetByID("t1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.State != StatePending || !reflect.DeepEqual(got.Logs, sampleLogs) {
		t.Errorf("unexpected stored task: %+v", got)
	}

	if _, err := proc.Submit("t1", sampleLogs); !errors.Is(err, ErrTaskExists) {
		t.Errorf("duplicate Submit err = %v, want ErrTaskExists", err)
	}
	if _, err := proc.Submit("", sampleLogs); !errors.Is(err, ErrEmptyTaskID) {
		t.Errorf("empty-id Submit err = %v, want ErrEmptyTaskID", err)
	}
}

func TestGetNonExistentTask(t *testing.T) {
	repo, proc := newTestEnv(t)

	if _, err := repo.GetByID("missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("GetByID err = %v, want ErrTaskNotFound", err)
	}
	if _, err := proc.ProcessByID("missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("ProcessByID err = %v, want ErrTaskNotFound", err)
	}
	ghost, _ := NewTask("ghost", sampleLogs)
	if err := repo.Save(ghost); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Save of never-created task err = %v, want ErrTaskNotFound", err)
	}
}

func TestProcessSuccess(t *testing.T) {
	repo, proc := newTestEnv(t)
	task, _ := proc.Submit("t1", sampleLogs)

	if err := proc.Process(task); err != nil {
		t.Fatalf("Process: %v", err)
	}

	stored, err := repo.GetByID("t1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	for name, got := range map[string]*Task{"caller": task, "stored": stored} {
		if got.State != StateCompleted {
			t.Errorf("%s state = %s, want COMPLETED", name, got.State)
		}
		if got.Result == nil {
			t.Fatalf("%s result is nil", name)
		}
		wantStats := map[LogLevel]int{LevelInfo: 2, LevelWarn: 1, LevelError: 2}
		if !reflect.DeepEqual(got.Result.Stats, wantStats) {
			t.Errorf("%s stats = %v, want %v", name, got.Result.Stats, wantStats)
		}
		if got.Result.TotalLines != 5 {
			t.Errorf("%s total = %d, want 5", name, got.Result.TotalLines)
		}
		wantAlerts := []Alert{
			{LineNumber: 2, Level: LevelError, Trigger: "timeout", Message: "DB connection timeout"},
			{LineNumber: 4, Level: LevelError, Trigger: "failed", Message: "Request failed after retry"},
			{LineNumber: 5, Level: LevelInfo, Trigger: "panic", Message: "Worker PANIC recovered"},
		}
		if !reflect.DeepEqual(got.Result.Alerts, wantAlerts) {
			t.Errorf("%s alerts = %+v, want %+v", name, got.Result.Alerts, wantAlerts)
		}
	}
}

func TestProcessAlertRules(t *testing.T) {
	tests := []struct {
		name       string
		triggers   []string
		logs       []string
		wantAlerts int
	}{
		{"no trigger words", nil, []string{"[INFO] 2026-10-15 all good"}, 0},
		{"level tag alone does not trigger", nil, []string{"[ERROR] 2026-10-15 something odd"}, 0},
		{"case-insensitive", nil, []string{"[WARN] 2026-10-15 TIMEOUT reached"}, 1},
		{"two triggers on one line", nil, []string{"[ERROR] 2026-10-15 timeout and failed"}, 2},
		{"custom trigger replaces defaults", []string{"disk"}, []string{"[WARN] 2026-10-15 disk full", "[ERROR] 2026-10-15 timeout"}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMemoryTaskRepository()
			proc, err := NewProcessor(repo, tc.triggers...)
			if err != nil {
				t.Fatal(err)
			}
			task, _ := proc.Submit("t", tc.logs)
			if err := proc.Process(task); err != nil {
				t.Fatalf("Process: %v", err)
			}
			if got := len(task.Result.Alerts); got != tc.wantAlerts {
				t.Errorf("alerts = %d, want %d (%+v)", got, tc.wantAlerts, task.Result.Alerts)
			}
		})
	}
}

func TestProcessInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		logs    []string
		wantErr error
	}{
		{"nil logs", nil, ErrEmptyBatch},
		{"empty logs", []string{}, ErrEmptyBatch},
		{"blank line", []string{"[INFO] 2026-10-15 ok", "   "}, ErrInvalidLogLine},
		{"unknown level", []string{"[FATAL] 2026-10-15 boom"}, ErrInvalidLogLine},
		{"bad date", []string{"[INFO] yesterday ok"}, ErrInvalidLogLine},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, proc := newTestEnv(t)
			task, _ := proc.Submit("t1", tc.logs)

			err := proc.Process(task)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}

			stored, _ := repo.GetByID("t1")
			if stored.State != StateFailed {
				t.Errorf("stored state = %s, want FAILED", stored.State)
			}
			if stored.FailureReason == "" {
				t.Errorf("FailureReason not recorded")
			}
			if stored.Result != nil {
				t.Errorf("failed task must not have a result")
			}
			if task.State != StateFailed {
				t.Errorf("caller's task state = %s, want FAILED", task.State)
			}
		})
	}
}

func TestProcessNilTask(t *testing.T) {
	_, proc := newTestEnv(t)
	if err := proc.Process(nil); !errors.Is(err, ErrNilTask) {
		t.Errorf("err = %v, want ErrNilTask", err)
	}
}

func TestProcessTaskNotInRepositoryLeavesCallerUntouched(t *testing.T) {
	_, proc := newTestEnv(t)
	orphan, _ := NewTask("orphan", sampleLogs)

	err := proc.Process(orphan)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
	if orphan.State != StatePending || orphan.Result != nil {
		t.Errorf("caller's task changed despite failed persistence: %+v", orphan)
	}
}

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to TaskState
		want     bool
	}{
		{StatePending, StateProcessing, true},
		{StatePending, StateFailed, true},
		{StatePending, StateCompleted, false},
		{StatePending, StatePending, false},

		{StateProcessing, StateCompleted, true},
		{StateProcessing, StateFailed, true},
		{StateProcessing, StatePending, false},
		{StateProcessing, StateProcessing, false},

		{StateCompleted, StatePending, false},
		{StateCompleted, StateProcessing, false},
		{StateCompleted, StateFailed, false},
		{StateCompleted, StateCompleted, false},

		{StateFailed, StatePending, false},
		{StateFailed, StateProcessing, false},
		{StateFailed, StateCompleted, false},
		{StateFailed, StateFailed, false},

		{TaskState("BOGUS"), StateProcessing, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
				t.Errorf("CanTransitionTo = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestTransitionToLeavesStateOnError(t *testing.T) {
	task, _ := NewTask("t1", sampleLogs)
	err := task.TransitionTo(StateCompleted)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if task.State != StatePending {
		t.Errorf("state changed to %s on a rejected transition", task.State)
	}
}

func TestCannotProcessCompletedTask(t *testing.T) {
	repo, proc := newTestEnv(t)
	task, _ := proc.Submit("t1", sampleLogs)
	if err := proc.Process(task); err != nil {
		t.Fatalf("first Process: %v", err)
	}
	before, _ := repo.GetByID("t1")

	if err := proc.Process(task); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("second Process err = %v, want ErrInvalidTransition", err)
	}
	if _, err := proc.ProcessByID("t1"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("ProcessByID err = %v, want ErrInvalidTransition", err)
	}

	after, _ := repo.GetByID("t1")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("stored task changed by a blocked transition")
	}
}

func TestCannotProcessFailedOrInFlightTask(t *testing.T) {
	_, proc := newTestEnv(t)

	failed, _ := proc.Submit("bad", []string{"garbage"})
	_ = proc.Process(failed)
	if failed.State != StateFailed {
		t.Fatalf("setup: state = %s, want FAILED", failed.State)
	}
	if err := proc.Process(failed); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("reprocessing failed task err = %v, want ErrInvalidTransition", err)
	}

	inFlight, _ := proc.Submit("busy", sampleLogs)
	inFlight.State = StateProcessing
	if err := proc.Process(inFlight); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("processing in-flight task err = %v, want ErrInvalidTransition", err)
	}
}

func TestMultipleTasksDoNotOverwriteEachOther(t *testing.T) {
	repo, proc := newTestEnv(t)

	logsA := []string{"[INFO] 2026-10-15 a1", "[INFO] 2026-10-15 a2"}
	logsB := []string{"[ERROR] 2026-10-15 b1 timeout"}
	logsC := []string{"[WARN] 2026-10-15 c1", "[WARN] 2026-10-15 c2", "[WARN] 2026-10-15 c3"}

	for id, logs := range map[string][]string{"A": logsA, "B": logsB, "C": logsC} {
		if _, err := proc.Submit(id, logs); err != nil {
			t.Fatalf("Submit %s: %v", id, err)
		}
	}

	for _, id := range []string{"A", "C"} {
		if _, err := proc.ProcessByID(id); err != nil {
			t.Fatalf("ProcessByID %s: %v", id, err)
		}
	}

	a, _ := repo.GetByID("A")
	b, _ := repo.GetByID("B")
	c, _ := repo.GetByID("C")

	if a.State != StateCompleted || a.Result.Stats[LevelInfo] != 2 || len(a.Result.Alerts) != 0 {
		t.Errorf("task A wrong: %+v / %+v", a, a.Result)
	}
	if c.State != StateCompleted || c.Result.Stats[LevelWarn] != 3 || c.Result.Stats[LevelInfo] != 0 {
		t.Errorf("task C wrong: %+v / %+v", c, c.Result)
	}
	if b.State != StatePending || b.Result != nil || !reflect.DeepEqual(b.Logs, logsB) {
		t.Errorf("task B was affected by others: %+v", b)
	}

	if _, err := proc.ProcessByID("B"); err != nil {
		t.Fatalf("ProcessByID B: %v", err)
	}
	a2, _ := repo.GetByID("A")
	c2, _ := repo.GetByID("C")
	b2, _ := repo.GetByID("B")
	if !reflect.DeepEqual(a, a2) || !reflect.DeepEqual(c, c2) {
		t.Errorf("processing B modified A or C")
	}
	if len(b2.Result.Alerts) != 1 || b2.Result.Stats[LevelError] != 1 {
		t.Errorf("task B result wrong: %+v", b2.Result)
	}
}

func TestRepositoryStoresIndependentCopies(t *testing.T) {
	repo, proc := newTestEnv(t)
	task, _ := proc.Submit("t1", sampleLogs)

	task.Logs[0] = "garbage"
	task.State = StateFailed

	stored, _ := repo.GetByID("t1")
	if stored.State != StatePending || stored.Logs[0] != sampleLogs[0] {
		t.Fatalf("stored task aliased the caller's task: %+v", stored)
	}

	stored.Logs[0] = "garbage"
	again, _ := repo.GetByID("t1")
	if again.Logs[0] != sampleLogs[0] {
		t.Errorf("GetByID returned a reference into the store")
	}

	fresh, _ := proc.Submit("t2", sampleLogs)
	if err := proc.Process(fresh); err != nil {
		t.Fatal(err)
	}
	fresh.Result.Stats[LevelInfo] = 999
	fresh.Result.Alerts[0].Trigger = "tampered"
	stored2, _ := repo.GetByID("t2")
	if stored2.Result.Stats[LevelInfo] != 2 || stored2.Result.Alerts[0].Trigger != "timeout" {
		t.Errorf("stored result aliased the caller's result: %+v", stored2.Result)
	}
}
