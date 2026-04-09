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

