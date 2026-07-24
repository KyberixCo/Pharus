package mcp

import (
	"fmt"
	"sync"
	"time"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

type ResearchTask struct {
	ID              string     `json:"id"`
	URI             string     `json:"uri"` // e.g. resource://tasks/{id}
	Topic           string     `json:"topic"`
	Status          TaskStatus `json:"status"`
	ProgressPercent int        `json:"progress_percent"`
	Phase           string     `json:"phase,omitempty"`
	Logs            []string   `json:"logs,omitempty"`
	Result          string     `json:"result,omitempty"`
	Error           string     `json:"error,omitempty"`
	UserID          string     `json:"user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type TaskRegistry struct {
	mu         sync.RWMutex
	tasks      map[string]*ResearchTask
	subManager *SubscriptionManager
}

func NewTaskRegistry(subManager ...*SubscriptionManager) *TaskRegistry {
	var sm *SubscriptionManager
	if len(subManager) > 0 {
		sm = subManager[0]
	}
	return &TaskRegistry{
		tasks:      make(map[string]*ResearchTask),
		subManager: sm,
	}
}

func (tr *TaskRegistry) SetSubscriptionManager(sm *SubscriptionManager) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.subManager = sm
}

func (tr *TaskRegistry) CreateTask(id string, topic string, userID ...string) *ResearchTask {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	now := time.Now()
	task := &ResearchTask{
		ID:              id,
		URI:             fmt.Sprintf("resource://tasks/%s", id),
		Topic:           topic,
		Status:          TaskStatusRunning,
		ProgressPercent: 0,
		Phase:           "initialized",
		Logs:            []string{fmt.Sprintf("[%s] Tarea creada para el tema: %s", now.Format(time.RFC3339), topic)},
		UserID:          uid,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	tr.tasks[id] = task

	if tr.subManager != nil {
		tr.subManager.Broadcast(EventTaskUpdated, task)
	}

	return task
}

func (tr *TaskRegistry) UpdateTaskProgress(id string, progress int, phase string, logMsg string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	task, exists := tr.tasks[id]
	if !exists {
		return
	}

	now := time.Now()
	task.ProgressPercent = progress
	if phase != "" {
		task.Phase = phase
	}
	if logMsg != "" {
		task.Logs = append(task.Logs, fmt.Sprintf("[%s] %s", now.Format(time.RFC3339), logMsg))
	}
	task.UpdatedAt = now

	if tr.subManager != nil {
		tr.subManager.Broadcast(EventTaskUpdated, task)
	}
}

func (tr *TaskRegistry) UpdateTaskStatus(id string, status TaskStatus, result string, errStr string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	task, exists := tr.tasks[id]
	if !exists {
		return
	}

	now := time.Now()
	task.Status = status
	task.Result = result
	task.Error = errStr
	if status == TaskStatusCompleted {
		task.ProgressPercent = 100
		task.Phase = "completed"
		task.Logs = append(task.Logs, fmt.Sprintf("[%s] Investigación completada con éxito", now.Format(time.RFC3339)))
	} else if status == TaskStatusFailed {
		task.Phase = "failed"
		task.Logs = append(task.Logs, fmt.Sprintf("[%s] Error: %s", now.Format(time.RFC3339), errStr))
	}
	task.UpdatedAt = now

	if tr.subManager != nil {
		tr.subManager.Broadcast(EventTaskUpdated, task)
	}
}

func (tr *TaskRegistry) GetTask(id string) (*ResearchTask, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	task, exists := tr.tasks[id]
	if !exists {
		return nil, false
	}
	// Devuelve una copia ligera
	tCopy := *task
	return &tCopy, true
}

func (tr *TaskRegistry) ListTasks(userID ...string) []*ResearchTask {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	var list []*ResearchTask
	for _, task := range tr.tasks {
		if uid == "" || task.UserID == "" || task.UserID == uid {
			tCopy := *task
			list = append(list, &tCopy)
		}
	}
	return list
}
