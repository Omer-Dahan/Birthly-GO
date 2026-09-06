package services

import (
	"strconv"
	"strings"
	"time"

	"birthly/internal/core"
	"birthly/internal/store/models"
)

var birthdayPrefixesHe = map[int]string{
	0:  "🎂 <b>היום יום ההולדת של {name}!</b>",
	1:  "🎁 מחר יום ההולדת של <b>{name}</b>",
	2:  "🎁 בעוד יומיים · יום ההולדת של <b>{name}</b>",
	7:  "🥳 בעוד שבוע · יום ההולדת של <b>{name}</b>",
	14: "🗓 בעוד שבועיים · יום ההולדת של <b>{name}</b>",
	30: "🗓 בעוד חודש · יום ההולדת של <b>{name}</b>",
}

var typeEmojisHe = map[string]string{
	core.EventTypeBirthday:    "🎂",
	core.EventTypeAnniversary: "💍",
	core.EventTypeWedding:     "💒",
	core.EventTypeMemorial:    "🕯",
	core.EventTypeCustom:      "📌",
}

var typeLabelsHe = map[string]string{
	core.EventTypeBirthday:    "יום הולדת",
	core.EventTypeAnniversary: "יום נישואין",
	core.EventTypeWedding:     "יום החתונה",
	core.EventTypeMemorial:    "אזכרה",
	core.EventTypeCustom:      "אירוע",
}

func displayName(event *models.Event) string {
	if event.LastName != nil && *event.LastName != "" {
		return event.FirstName + " " + *event.LastName
	}
	return event.FirstName
}

func genderPhrase(event *models.Event, lang string) string {
	if lang == core.LanguageHe {
		if event.Gender != nil {
			switch *event.Gender {
			case core.GenderMale:
				return "הוא חוגג"
			case core.GenderFemale:
				return "היא חוגגת"
			}
		}
		return "חוגג/ת"
	}
	return "celebrating"
}

// trackMarkerLine names which calendar track a reminder fires for, so a
// dual-date event's two reminders don't read as duplicates of each other.
// Only shown when the event actually has a secondary date configured.
func trackMarkerLine(trackCalendarType, lang string) string {
	isHebrew := trackCalendarType == core.CalendarTypeHebrew
	if lang == core.LanguageHe {
		if isHebrew {
			return "🗓 לפי הלוח העברי"
		}
		return "🗓 לפי הלוח הלועזי"
	}
	if isHebrew {
		return "🗓 Hebrew calendar date"
	}
	return "🗓 Gregorian calendar date"
}

// RenderReminder builds the HTML reminder message text (SPEC.md S16).
// occurrence is the actual date this reminder fires for (the track's own
// next_occurrence). trackCalendarType identifies which calendar track fired
// ("hebrew" or "gregorian"): only surfaced in the text when the event has a
// secondary date, so a single-date event's message is unchanged.
func RenderReminder(user *models.User, event *models.Event, rule *models.ReminderRule, occurrence time.Time, trackCalendarType string) string {
	name := displayName(event)
	lang := user.Language
	offset := 0
	if rule != nil && rule.OffsetDays != nil {
		offset = *rule.OffsetDays
	}
	etype := event.EventType

	var header string
	if lang == core.LanguageHe {
		switch {
		case etype == core.EventTypeBirthday:
			if tpl, ok := birthdayPrefixesHe[offset]; ok {
				header = strings.ReplaceAll(tpl, "{name}", name)
			} else {
				header = "📅 בעוד " + strconv.Itoa(offset) + " ימים · יום ההולדת של <b>" + name + "</b>"
			}
		case etype == core.EventTypeMemorial:
			header = typeEmojisHe[core.EventTypeMemorial] + " <b>אזכרה · " + name + "</b>"
		default:
			emoji := typeEmojisHe[etype]
			if emoji == "" {
				emoji = "📌"
			}
			label := typeLabelsHe[etype]
			if event.CustomTypeLabel != nil && *event.CustomTypeLabel != "" {
				label = *event.CustomTypeLabel
			} else if label == "" {
				label = etype
			}
			header = emoji + " <b>" + label + " · " + name + "</b>"
		}
	} else {
		header = "🎂 <b>" + name + "</b>'s birthday"
		if offset == 1 {
			header = "🎁 Tomorrow is <b>" + name + "</b>'s birthday"
		} else if offset > 1 {
			header = "📅 In " + strconv.Itoa(offset) + " days · <b>" + name + "</b>'s birthday"
		}
	}

	lines := []string{header}
	if event.SecondaryMonth != nil {
		lines = append(lines, trackMarkerLine(trackCalendarType, lang))
	}
	lines = append(lines, "")

	if etype == core.EventTypeBirthday {
		if age, ok := core.AgeAt(event.CalendarType, event.Year, occurrence); ok {
			phrase := genderPhrase(event, lang)
			if lang == core.LanguageHe {
				lines = append(lines, "🎈 "+phrase+" <b>"+strconv.Itoa(age)+"</b>")
			} else {
				lines = append(lines, "🎈 Turning <b>"+strconv.Itoa(age)+"</b>")
			}
		}
	}

	relation := event.Relation != nil && *event.Relation != ""
	if relation && event.Category != "" && event.Category != core.CategoryOther {
		lines = append(lines, "🏷 "+event.Category+" · "+*event.Relation)
	} else if relation {
		lines = append(lines, "🏷 "+*event.Relation)
	}

	if event.Phone != nil && *event.Phone != "" {
		lines = append(lines, "📞 "+*event.Phone)
	}

	return strings.Join(lines, "\n")
}
