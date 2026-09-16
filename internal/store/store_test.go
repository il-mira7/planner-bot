package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAddDonePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 17, 18, 0, 0, 0, time.UTC)
	a := st.Add("сдать тестовое", &due)
	st.Add("без дедлайна", nil)
	if got := len(st.Active()); got != 2 {
		t.Fatalf("ожидала 2 активные, got %d", got)
	}
	if _, err := st.Done(a.ID); err != nil {
		t.Fatal(err)
	}
	if got := len(st.Active()); got != 1 {
		t.Fatalf("после /done ожидала 1 активную, got %d", got)
	}

	// переоткрытие: данные должны пережить рестарт
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st2.Active()); got != 1 {
		t.Fatalf("после перезапуска ожидала 1 активную, got %d", got)
	}
	if st2.Active()[0].Text != "без дедлайна" {
		t.Fatalf("не та задача: %+v", st2.Active()[0])
	}
}

func TestActiveOrderByDue(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	late := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	soon := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	st.Add("поздняя", &late)
	st.Add("без даты", nil)
	st.Add("скорая", &soon)
	tasks := st.Active()
	if tasks[0].Text != "скорая" || tasks[1].Text != "поздняя" || tasks[2].Text != "без даты" {
		t.Fatalf("порядок сломан: %+v", tasks)
	}
}

func TestDeleteMissing(t *testing.T) {
	st, _ := Open(filepath.Join(t.TempDir(), "tasks.json"))
	if err := st.Delete(99); err != ErrNotFound {
		t.Fatalf("ожидала ErrNotFound, got %v", err)
	}
}
