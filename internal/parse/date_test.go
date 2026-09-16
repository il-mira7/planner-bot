package parse

import (
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) // среда

func TestExtractDue(t *testing.T) {
	cases := []struct {
		in        string
		wantDue   time.Time
		wantText  string
		wantFound bool
	}{
		{"полить цветы завтра в 18", time.Date(2026, 9, 17, 18, 0, 0, 0, time.UTC), "полить цветы", true},
		{"созвон 15.10 в 14:30", time.Date(2026, 10, 15, 14, 30, 0, 0, time.UTC), "созвон", true},
		{"позвонить в 19", time.Date(2026, 9, 16, 19, 0, 0, 0, time.UTC), "позвонить", true},
		{"сдать тестовое через 2 дня", time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC), "сдать тестовое", true},
		{"купить хлеб сегодня", time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC), "купить хлеб", true},
		{"встреча послезавтра в 10", time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC), "встреча", true},
		{"перезвонить через 30 минут", fixedNow.Add(30 * time.Minute), "перезвонить", true},
		{"в 9", time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC), "", true},
		{"просто задача без дат", time.Time{}, "просто задача без дат", false},
		{"отнести документы 3.01", time.Date(2027, 1, 3, 9, 0, 0, 0, time.UTC), "отнести документы", true},
	}
	for _, c := range cases {
		due, text, found := ExtractDue(c.in, fixedNow)
		if found != c.wantFound {
			t.Errorf("%q: found=%v, хочу %v", c.in, found, c.wantFound)
			continue
		}
		if !c.wantFound {
			continue
		}
		if !due.Equal(c.wantDue) {
			t.Errorf("%q: due=%v, хочу %v", c.in, due, c.wantDue)
		}
		if text != c.wantText {
			t.Errorf("%q: text=%q, хочу %q", c.in, text, c.wantText)
		}
	}
}
