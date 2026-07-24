package mcp

import (
	"testing"
	"time"
)

func TestTaskRegistryAndSubscriptions(t *testing.T) {
	sm := NewSubscriptionManager()
	subID := "test_sub"
	ch := sm.Subscribe(subID)
	defer sm.Unsubscribe(subID)

	tr := NewTaskRegistry(sm)

	// Create task
	task := tr.CreateTask("task_1", "Tema de prueba", "user_123")
	if task.ID != "task_1" || task.UserID != "user_123" {
		t.Fatalf("Tarea no inicializada correctamente: %+v", task)
	}

	// Verify broadcast event for task creation
	select {
	case msg := <-ch:
		if msg.Type != EventTaskUpdated {
			t.Errorf("Esperado evento %s, obtenido %s", EventTaskUpdated, msg.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("No se recibió evento broadcast al crear tarea")
	}

	// Update progress
	tr.UpdateTaskProgress("task_1", 50, "searxng_search", "Buscando en SearXNG")
	tCheck, ok := tr.GetTask("task_1")
	if !ok || tCheck.ProgressPercent != 50 || tCheck.Phase != "searxng_search" {
		t.Errorf("Progreso de tarea no actualizado correctamente: %+v", tCheck)
	}

	// Complete task
	tr.UpdateTaskStatus("task_1", TaskStatusCompleted, "Reporte final de prueba", "")
	tDone, _ := tr.GetTask("task_1")
	if tDone.Status != TaskStatusCompleted || tDone.ProgressPercent != 100 || tDone.Result != "Reporte final de prueba" {
		t.Errorf("Estado completado incorrecto: %+v", tDone)
	}

	// List tasks with filtering
	tr.CreateTask("task_2", "Otro tema", "user_456")
	u1Tasks := tr.ListTasks("user_123")
	if len(u1Tasks) != 1 || u1Tasks[0].ID != "task_1" {
		t.Errorf("Filtrado por UserID falló: esperada 1 tarea, obtenidas %d", len(u1Tasks))
	}
}
