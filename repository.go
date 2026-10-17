package main

import "fmt"

type TaskRepository interface {
	Create(task *Task) error
	Save(task *Task) error
	GetByID(id string) (*Task, error)
}

type MemoryTaskRepository struct {
	tasks map[string]*Task
}

var _ TaskRepository = (*MemoryTaskRepository)(nil)

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
