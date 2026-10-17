package main

import "fmt"

// TaskRepository is the storage abstraction the processor depends on.
//
// Design note (interfaces): the processor needs only these three behaviours,
// so it depends on this small interface instead of a concrete type. That keeps
// it decoupled from the storage mechanism: the in-memory map below can later
// be swapped for a different implementation (or a test fake) without touching
// processor code. The interface is kept small on purpose - every extra method
// is something each future implementation must support.
type TaskRepository interface {
	// Create stores a new task. It fails with ErrTaskExists if the ID is taken.
	Create(task *Task) error
	// Save overwrites an existing task. It fails with ErrTaskNotFound if the
	// task was never created.
	Save(task *Task) error
	// GetByID returns the task with the given ID, or ErrTaskNotFound.
	GetByID(id string) (*Task, error)
}

// MemoryTaskRepository is a map-backed TaskRepository.
//
// Design note (copies): the map holds *Task, but every method copies tasks on
// the way in and on the way out. Without this, the caller's pointer and the
// stored pointer would be the same object, so editing a task would change
// stored data without calling Save, and "Save" would be meaningless. With
// copies, the repository is the only owner of what it stores.
//
// It is NOT safe for concurrent use. Checkpoint 2 will add synchronisation.
type MemoryTaskRepository struct {
	tasks map[string]*Task
}

// Compile-time proof that the concrete type satisfies the interface.
var _ TaskRepository = (*MemoryTaskRepository)(nil)

// NewMemoryTaskRepository returns an empty repository. A constructor is needed
// because the zero value would hold a nil map, and writing to a nil map panics.
func NewMemoryTaskRepository() *MemoryTaskRepository {
	return &MemoryTaskRepository{tasks: make(map[string]*Task)}
}

func (r *MemoryTaskRepository) Create(task *Task) error {
	if task == nil {
		return ErrNilTask
	}
	if task.ID == "" {
		return ErrEmptyTaskID
	}
	if _, exists := r.tasks[task.ID]; exists {
		return fmt.Errorf("task %q: %w", task.ID, ErrTaskExists)
	}
	r.tasks[task.ID] = task.Clone()
	return nil
}

func (r *MemoryTaskRepository) Save(task *Task) error {
	if task == nil {
		return ErrNilTask
	}
	if _, exists := r.tasks[task.ID]; !exists {
		return fmt.Errorf("task %q: %w", task.ID, ErrTaskNotFound)
	}
	r.tasks[task.ID] = task.Clone()
	return nil
}

func (r *MemoryTaskRepository) GetByID(id string) (*Task, error) {
	task, exists := r.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task %q: %w", id, ErrTaskNotFound)
	}
	return task.Clone(), nil
}
