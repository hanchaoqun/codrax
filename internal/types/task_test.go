package types

import "testing"

func TestTaskList_CurrentTask(t *testing.T) {
	tl := &TaskList{
		Objective:     "test objective",
		CurrentTaskID: "t2",
		Tasks: []TaskItem{
			{ID: "t1", Title: "Task 1", Status: TaskPending},
			{ID: "t2", Title: "Task 2", Status: TaskInProgress},
			{ID: "t3", Title: "Task 3", Status: TaskPending},
		},
	}

	t.Run("matching CurrentTaskID", func(t *testing.T) {
		task := tl.CurrentTask()
		if task == nil {
			t.Fatal("expected non-nil task")
		}
		if task.ID != "t2" {
			t.Fatalf("expected task ID t2, got %s", task.ID)
		}
		if task.Title != "Task 2" {
			t.Fatalf("expected title Task 2, got %s", task.Title)
		}
	})

	t.Run("non-matching CurrentTaskID", func(t *testing.T) {
		tl2 := &TaskList{
			CurrentTaskID: "nonexistent",
			Tasks: []TaskItem{
				{ID: "t1", Title: "Task 1", Status: TaskPending},
			},
		}
		task := tl2.CurrentTask()
		if task != nil {
			t.Fatalf("expected nil task, got %+v", task)
		}
	})
}

// TestUpdateTaskResult_MatchingID covers the normal path: the supplied
// ID matches a task, result/status are written there, the returned ID
// equals the supplied ID.
func TestUpdateTaskResult_MatchingID(t *testing.T) {
	m := NewMutableState(TaskList{
		Tasks: []TaskItem{
			{ID: "t1", Status: TaskPending},
			{ID: "t2", Status: TaskInProgress},
		},
	})
	got := m.UpdateTaskResult("t2", "hello", TaskDone)
	if got != "t2" {
		t.Errorf("returned ID = %q, want t2", got)
	}
	tl := m.TaskList()
	if tl.Tasks[1].Result != "hello" || tl.Tasks[1].Status != TaskDone {
		t.Errorf("task t2 not updated: %+v", tl.Tasks[1])
	}
}

// TestUpdateTaskResult_StaleIDFallsBackToInProgress covers S1: when
// the supplied ID is missing from the list (the scenario df3 run 3
// triggered by an analyzer todo_write replace), the update falls back
// to the first in_progress task so the finalizer answer is not
// silently dropped. The returned ID reflects the actual write target
// so the caller can log a warning.
func TestUpdateTaskResult_StaleIDFallsBackToInProgress(t *testing.T) {
	m := NewMutableState(TaskList{
		Tasks: []TaskItem{
			{ID: "new-1", Status: TaskPending},
			{ID: "new-2", Status: TaskInProgress},
		},
	})
	got := m.UpdateTaskResult("stale-id", "final answer", TaskDone)
	if got != "new-2" {
		t.Errorf("returned ID = %q, want new-2 (the in_progress task)", got)
	}
	tl := m.TaskList()
	if tl.Tasks[1].Result != "final answer" {
		t.Errorf("fallback target not updated: %+v", tl.Tasks[1])
	}
	// Non-target must not be touched.
	if tl.Tasks[0].Result != "" {
		t.Errorf("non-target mutated: %+v", tl.Tasks[0])
	}
}

// TestUpdateTaskResult_StaleIDFallsBackToPending verifies fallback
// preference: when no in_progress task exists, the first pending
// task receives the result.
func TestUpdateTaskResult_StaleIDFallsBackToPending(t *testing.T) {
	m := NewMutableState(TaskList{
		Tasks: []TaskItem{
			{ID: "done", Status: TaskDone, Result: "already"},
			{ID: "pend-1", Status: TaskPending},
			{ID: "pend-2", Status: TaskPending},
		},
	})
	got := m.UpdateTaskResult("stale", "x", TaskDone)
	if got != "pend-1" {
		t.Errorf("returned ID = %q, want pend-1", got)
	}
	if m.TaskList().Tasks[0].Result != "already" {
		t.Errorf("done task should be untouched")
	}
}

// TestUpdateTaskResult_StaleIDFallsBackToFirst verifies the last-
// resort fallback when all tasks are done: the first task is reused.
func TestUpdateTaskResult_StaleIDFallsBackToFirst(t *testing.T) {
	m := NewMutableState(TaskList{
		Tasks: []TaskItem{
			{ID: "a", Status: TaskDone},
			{ID: "b", Status: TaskDone},
		},
	})
	got := m.UpdateTaskResult("stale", "x", TaskDone)
	if got != "a" {
		t.Errorf("returned ID = %q, want a", got)
	}
}

// TestUpdateTaskResult_EmptyListReturnsEmpty verifies the "no tasks
// at all" edge case: returns empty string so the caller can log a
// hard-drop warning.
func TestUpdateTaskResult_EmptyListReturnsEmpty(t *testing.T) {
	m := NewMutableState(TaskList{})
	got := m.UpdateTaskResult("anything", "x", TaskDone)
	if got != "" {
		t.Errorf("returned ID = %q, want empty", got)
	}
}

