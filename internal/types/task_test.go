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

func TestTaskList_UpdateTask(t *testing.T) {
	tl := &TaskList{
		Tasks: []TaskItem{
			{ID: "t1", Title: "Task 1", Status: TaskPending},
			{ID: "t2", Title: "Task 2", Status: TaskInProgress},
		},
	}

	t.Run("update status and result", func(t *testing.T) {
		tl.UpdateTask("t1", TaskDone, "completed successfully")

		if tl.Tasks[0].Status != TaskDone {
			t.Fatalf("expected status %s, got %s", TaskDone, tl.Tasks[0].Status)
		}
		if tl.Tasks[0].Result != "completed successfully" {
			t.Fatalf("expected result 'completed successfully', got %q", tl.Tasks[0].Result)
		}
	})

	t.Run("update nonexistent task is no-op", func(t *testing.T) {
		// Should not panic or change anything.
		tl.UpdateTask("nonexistent", TaskFailed, "error")
		if tl.Tasks[0].Status != TaskDone {
			t.Fatalf("task t1 status changed unexpectedly: %s", tl.Tasks[0].Status)
		}
		if tl.Tasks[1].Status != TaskInProgress {
			t.Fatalf("task t2 status changed unexpectedly: %s", tl.Tasks[1].Status)
		}
	})
}

func TestTaskList_NextPendingTask(t *testing.T) {
	t.Run("returns first pending, skips in_progress", func(t *testing.T) {
		tl := &TaskList{
			Tasks: []TaskItem{
				{ID: "t1", Title: "Task 1", Status: TaskInProgress},
				{ID: "t2", Title: "Task 2", Status: TaskPending},
				{ID: "t3", Title: "Task 3", Status: TaskPending},
			},
		}
		task := tl.NextPendingTask()
		if task == nil {
			t.Fatal("expected non-nil task")
		}
		if task.ID != "t2" {
			t.Fatalf("expected task ID t2, got %s", task.ID)
		}
	})

	t.Run("returns nil when no pending tasks", func(t *testing.T) {
		tl := &TaskList{
			Tasks: []TaskItem{
				{ID: "t1", Status: TaskDone},
				{ID: "t2", Status: TaskFailed},
				{ID: "t3", Status: TaskInProgress},
			},
		}
		task := tl.NextPendingTask()
		if task != nil {
			t.Fatalf("expected nil, got task %s", task.ID)
		}
	})
}

func TestTaskList_AllDone(t *testing.T) {
	t.Run("all done or failed returns true", func(t *testing.T) {
		tl := &TaskList{
			Tasks: []TaskItem{
				{ID: "t1", Status: TaskDone},
				{ID: "t2", Status: TaskFailed},
				{ID: "t3", Status: TaskDone},
			},
		}
		if !tl.AllDone() {
			t.Fatal("expected AllDone to return true")
		}
	})

	t.Run("pending task returns false", func(t *testing.T) {
		tl := &TaskList{
			Tasks: []TaskItem{
				{ID: "t1", Status: TaskDone},
				{ID: "t2", Status: TaskPending},
			},
		}
		if tl.AllDone() {
			t.Fatal("expected AllDone to return false")
		}
	})

	t.Run("in_progress task returns false", func(t *testing.T) {
		tl := &TaskList{
			Tasks: []TaskItem{
				{ID: "t1", Status: TaskDone},
				{ID: "t2", Status: TaskInProgress},
			},
		}
		if tl.AllDone() {
			t.Fatal("expected AllDone to return false")
		}
	})

	t.Run("empty task list returns true", func(t *testing.T) {
		tl := &TaskList{Tasks: []TaskItem{}}
		if !tl.AllDone() {
			t.Fatal("expected AllDone to return true for empty list")
		}
	})
}
