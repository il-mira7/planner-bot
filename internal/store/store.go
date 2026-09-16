// Package store — простое персистентное хранилище задач в JSON-файле.
// Рассчитано на одного пользователя, запись атомарная (tmp + rename).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Task struct {
	ID        int        `json:"id"`
	Text      string     `json:"text"`
	Due       *time.Time `json:"due,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Done      bool       `json:"done"`
	Notified  bool       `json:"notified,omitempty"` // напоминание о дедлайне отправлено
}

type data struct {
	Tasks     []Task `json:"tasks"`
	NextID    int    `json:"next_id"`
	ChatID    int64  `json:"chat_id,omitempty"`    // чат владельца для напоминаний
	LastBrief string `json:"last_brief,omitempty"` // дата последней утренней сводки
}

type Store struct {
	mu   sync.Mutex
	path string
	d    data
}

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.d.NextID = 1
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.d); err != nil {
		return nil, fmt.Errorf("не удалось разобрать %s: %w", path, err)
	}
	if s.d.NextID == 0 {
		s.d.NextID = 1
	}
	return s, nil
}

// Add создаёт задачу и возвращает её.
func (s *Store) Add(text string, due *time.Time) Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.d.NextID, Text: text, Due: due, CreatedAt: time.Now()}
	s.d.NextID++
	s.d.Tasks = append(s.d.Tasks, t)
	_ = s.save()
	return t
}

var ErrNotFound = errors.New("задача не найдена")

func (s *Store) Done(id int) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.Tasks {
		if s.d.Tasks[i].ID == id {
			s.d.Tasks[i].Done = true
			_ = s.save()
			return s.d.Tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.Tasks {
		if s.d.Tasks[i].ID == id {
			s.d.Tasks = append(s.d.Tasks[:i], s.d.Tasks[i+1:]...)
			_ = s.save()
			return nil
		}
	}
	return ErrNotFound
}

// Active возвращает незакрытые задачи, отсортированные по дедлайну
// (без дедлайна — в конец, по времени создания).
func (s *Store) Active() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Task
	for _, t := range s.d.Tasks {
		if !t.Done {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Due != nil && b.Due != nil {
			return a.Due.Before(*b.Due)
		}
		return a.Due != nil
	})
	return out
}

// MarkNotified помечает задачу как напомнённую (и сохраняет).
func (s *Store) MarkNotified(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.Tasks {
		if s.d.Tasks[i].ID == id {
			s.d.Tasks[i].Notified = true
			_ = s.save()
			return
		}
	}
}

// SetChat запоминает чат владельца, если ещё не задан.
func (s *Store) SetChat(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.ChatID == 0 {
		s.d.ChatID = id
		_ = s.save()
	}
}

func (s *Store) ChatID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.ChatID
}

// ShouldBrief сообщает, пора ли утренняя сводка (окно 09:00–12:00), и фиксирует дату.
func (s *Store) ShouldBrief(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := now.Format("2006-01-02")
	if s.d.LastBrief == today {
		return false
	}
	if now.Hour() < 9 || now.Hour() >= 12 {
		return false
	}
	s.d.LastBrief = today
	_ = s.save()
	return true
}

func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
