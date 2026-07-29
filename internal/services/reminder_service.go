package services

import (
	"strconv"
	"strings"

	"birthly/internal/core"
	"birthly/internal/store/models"
)

var offsetLabelsHe = map[int]string{
	0: "ביום עצמו", 1: "יום לפני", 2: "יומיים לפני", 3: "3 ימים לפני",
	7: "שבוע לפני", 14: "שבועיים לפני", 30: "חודש לפני",
}

var offsetLabelsEn = map[int]string{
	0: "the day of", 1: "the day before", 2: "2 days before", 3: "3 days before",
	7: "a week before", 14: "two weeks before", 30: "a month before",
}

// OffsetLabel renders a reminder offset (days before) in lang.
func OffsetLabel(offsetDays int, lang string) string {
	labels := offsetLabelsEn
	if lang == core.LanguageHe {
		labels = offsetLabelsHe
	}
	if label, ok := labels[offsetDays]; ok {
		return label
	}
	return strconv.Itoa(offsetDays) + "d"
}

func describe(label, sendTime, lang string) string {
	if lang == core.LanguageHe {
		return label + ", ב-" + sendTime
	}
	return label + ", at " + sendTime
}

// DescribeRule renders e.g. "יום לפני, ב-09:00" — used in confirmations and
// card summaries.
func DescribeRule(rule *models.ReminderRule, user *models.User) string {
	sendTime := user.DefaultNotifyTime
	if rule.SendTime != nil {
		sendTime = *rule.SendTime
	}
	if rule.OffsetDays == nil {
		return sendTime
	}
	label := OffsetLabel(*rule.OffsetDays, user.Language)
	return describe(label, sendTime, user.Language)
}

// DescribeDefaultReminder is the confirmation text shown right after adding
// an event (S5): describes the "day before" default rule every new user
// starts with.
func DescribeDefaultReminder(user *models.User) string {
	label := OffsetLabel(1, user.Language)
	return describe(label, user.DefaultNotifyTime, user.Language)
}

// SummarizeRules renders short joined labels for a card, e.g.
// "יום לפני · ביום עצמו". Empty if no rules.
func SummarizeRules(rules []*models.ReminderRule, lang string) string {
	var labels []string
	for _, r := range rules {
		if !r.Enabled || r.OffsetDays == nil {
			continue
		}
		labels = append(labels, OffsetLabel(*r.OffsetDays, lang))
	}
	return strings.Join(labels, " · ")
}
