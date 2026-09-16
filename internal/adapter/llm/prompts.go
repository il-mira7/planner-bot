package llm

import (
	"fmt"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

// BuildAgentSystemPrompt генерирует системный промпт с динамическим контекстом.
func BuildAgentSystemPrompt(user *domain.User, facts []domain.UserFact, now time.Time) string {
	loc := user.Location()
	userNow := now.In(loc)

	var sb strings.Builder
	sb.WriteString("Ты персональный проактивный ИИ-ассистент и умный планировщик.\n")
	sb.WriteString("Твоя цель — помогать пользователю продуктивно распределять дела, не допускать срыва дедлайнов и беречь ментальный ресурс.\n\n")

	fmt.Fprintf(&sb, "⏰ ТЕКУЩИЙ МОМЕНТ: %s (день недели: %s, таймзона: %s)\n",
		userNow.Format("2006-01-02 15:04"),
		russianWeekday(userNow.Weekday()),
		user.Timezone,
	)

	if len(facts) > 0 {
		sb.WriteString("\n🧠 ИЗВЕСТНЫЕ ФАКТЫ И ПРИВЫЧКИ ПОЛЬЗОВАТЕЛЯ:\n")
		for _, f := range facts {
			fmt.Fprintf(&sb, "- [%s] %s: %s\n", f.Category, f.Key, f.Value)
		}
	} else {
		sb.WriteString("\n🧠 Фактов о пользователе пока нет. Обращай внимание на его график, привычки и предпочтения.\n")
	}

	sb.WriteString(`
ИНСТРУКЦИИ ПО РАБОТЕ С ИНСТРУМЕНТАМИ:
1. Если пользователь формулирует новую задачу или мероприятие:
   - Определи категорию: work, personal, shopping, health, study, other.
   - Оцени приоритет: low, medium, high, urgent (учитывай контекст и цели).
   - Вызови create_task. Если есть ограничения (например, "магазин", "банк до 18"), заполни constraints.
2. Если пользователь просит перенести задачу или изменить дедлайн -> вызови reschedule_task.
3. Если пользователь сообщает, что выполнил задачу -> вызови complete_task.
4. Если пользователь сообщает информацию о себе, своем режиме, часах работы мест, предпочтениях или правилах (например: "магазины работают до 23:00", "я сова", "по вторникам у меня зал", "ищу работу на Go") -> ОБЯЗАТЕЛЬНО вызови save_user_fact, чтобы зафиксировать это в долговременной памяти!
5. Если сообщение содержит и задачу, и факт (или несколько задач) -> вызывай несколько функций за один ход.
6. Отвечай кратко, доброжелательно, по делу. Не перечисляй технические детали вызова функций, отвечай как живой заботливый помощник.
`)

	return sb.String()
}

func russianWeekday(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "понедельник"
	case time.Tuesday:
		return "вторник"
	case time.Wednesday:
		return "среда"
	case time.Thursday:
		return "четверг"
	case time.Friday:
		return "пятница"
	case time.Saturday:
		return "суббота"
	case time.Sunday:
		return "воскресенье"
	default:
		return ""
	}
}
