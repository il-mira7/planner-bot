// Package parse вытаскивает дедлайн из задачи на естественном языке:
// «завтра в 18», «через 2 дня», «15.10 в 14:30». Работает без сети.
package parse

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type span struct{ start, end int }

// Границы слов проверяются в коде (unicode.IsLetter), потому что
// \b в Go-регулярках определён только для ASCII и не видит кириллицу.
var (
	dayRe     = regexp.MustCompile(`(?i)послезавтра|завтра|сегодня`)
	dateRe    = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?\b`)
	timeRe    = regexp.MustCompile(`(?i)в\s+(\d{1,2})(?::(\d{2}))?`)
	throughRe = regexp.MustCompile(`(?i)через\s+(полтора|пару|\d+)\s*([а-яё]*)`)
)

// ExtractDue ищет дедлайн в тексте задачи и возвращает его,
// очищенный текст и признак успеха.
func ExtractDue(text string, now time.Time) (time.Time, string, bool) {
	var removals []span
	var dayMatch string
	var dateY, dateM, dateD int
	haveDate := false
	hh, mm := -1, -1
	haveThrough := false
	var throughAt time.Time

	if m := throughRe.FindStringSubmatchIndex(text); m != nil && startsClean(text, m[0]) && endsClean(text, m[1]) {
		haveThrough = true
		throughAt = throughTime(text[m[2]:m[3]], text[m[4]:m[5]], now)
		removals = append(removals, span{m[0], m[1]})
	}

	if !haveThrough {
		if m := dateRe.FindStringSubmatchIndex(text); m != nil {
			var dErr, mErr error
			dateD, dErr = strconv.Atoi(text[m[2]:m[3]])
			dateM, mErr = strconv.Atoi(text[m[4]:m[5]])
			if dErr == nil && mErr == nil && dateD >= 1 && dateD <= 31 && dateM >= 1 && dateM <= 12 {
				haveDate = true
				if m[6] >= 0 {
					y := text[m[6]:m[7]]
					if len(y) == 2 {
						y = "20" + y
					}
					dateY, _ = strconv.Atoi(y)
				}
				removals = append(removals, span{m[0], m[1]})
			}
		}
		if m := dayRe.FindStringSubmatchIndex(text); m != nil && startsClean(text, m[0]) && endsClean(text, m[1]) {
			dayMatch = strings.ToLower(text[m[0]:m[1]])
			removals = append(removals, span{m[0], m[1]})
		}
		if m := timeRe.FindStringSubmatchIndex(text); m != nil && startsClean(text, m[0]) && timeEndsClean(text, m[1]) && !overlaps(removals, m[0], m[1]) {
			h, _ := strconv.Atoi(text[m[2]:m[3]])
			min := 0
			if m[4] >= 0 {
				min, _ = strconv.Atoi(text[m[4]:m[5]])
			}
			if h <= 23 && min <= 59 {
				hh, mm = h, min
				removals = append(removals, span{m[0], m[1]})
			}
		}
	}

	if !haveThrough && !haveDate && dayMatch == "" && hh < 0 {
		return time.Time{}, text, false
	}

	var due time.Time
	switch {
	case haveThrough:
		due = throughAt
	case haveDate:
		year := dateY
		if year == 0 {
			year = now.Year()
		}
		due = time.Date(year, time.Month(dateM), dateD, 0, 0, 0, 0, now.Location())
		if due.Before(now) && dateY == 0 {
			due = due.AddDate(1, 0, 0) // дата без года уже прошла — значит, в следующем
		}
		applyTime(&due, hh, mm, 9, 0)
	case dayMatch != "":
		switch dayMatch {
		case "сегодня":
			due = truncateToDay(now)
			applyTime(&due, hh, mm, 18, 0)
		case "завтра":
			due = truncateToDay(now.AddDate(0, 0, 1))
			applyTime(&due, hh, mm, 9, 0)
		case "послезавтра":
			due = truncateToDay(now.AddDate(0, 0, 2))
			applyTime(&due, hh, mm, 9, 0)
		}
	default: // только время «в 19»
		due = truncateToDay(now)
		applyTime(&due, hh, mm, 0, 0)
		if !due.After(now) {
			due = due.AddDate(0, 0, 1)
		}
	}

	return due, clean(text, removals), true
}

func applyTime(t *time.Time, hh, mm, defH, defM int) {
	if hh >= 0 {
		defH, defM = hh, mm
	}
	*t = time.Date(t.Year(), t.Month(), t.Day(), defH, defM, 0, 0, t.Location())
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func throughTime(num, unit string, now time.Time) time.Time {
	n := 1.0
	switch strings.ToLower(num) {
	case "пару":
		n = 2
	case "полтора":
		n = 1.5
	default:
		var i int
		i, _ = strconv.Atoi(strings.ToLower(num))
		n = float64(i)
	}
	u := strings.ToLower(unit)
	// «через 2 дня» — считаем утром (09:00), «через 30 минут» — точное время
	switch {
	case strings.HasPrefix(u, "мин"):
		return now.Add(time.Duration(n * float64(time.Minute)))
	case u == "ч" || strings.HasPrefix(u, "ча"):
		return now.Add(time.Duration(n * float64(time.Hour)))
	case strings.HasPrefix(u, "мес"):
		return truncateToDay(now.AddDate(0, int(n), 0)).Add(9 * time.Hour)
	case u == "дн" || u == "д" || strings.HasPrefix(u, "дн") || strings.HasPrefix(u, "де"):
		return truncateToDay(now.AddDate(0, 0, int(n))).Add(9 * time.Hour)
	case strings.HasPrefix(u, "нед"):
		return truncateToDay(now.AddDate(0, 0, int(n)*7)).Add(9 * time.Hour)
	default: // единица не указана — считаем часами
		return now.Add(time.Duration(n * float64(time.Hour)))
	}
}

// startsClean — совпадение не начинается в середине слова.
func startsClean(text string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:i])
	return !unicode.IsLetter(r)
}

// endsClean — совпадение не заканчивается в середине слова.
func endsClean(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return !unicode.IsLetter(r)
}

// timeEndsClean — после времени не должно быть цифры или точки («в 15.10»).
func timeEndsClean(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.'
}

func overlaps(spans []span, start, end int) bool {
	for _, s := range spans {
		if start < s.end && end > s.start {
			return true
		}
	}
	return false
}

func clean(text string, removals []span) string {
	mask := []byte(text)
	for _, s := range removals {
		for i := s.start; i < s.end && i < len(mask); i++ {
			mask[i] = ' '
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(string(mask)), " "))
}
