package core

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Escape sequences, not raw literal characters: an invisible bidi control
// character sitting directly in source text is a landmine for editors, git
// diffs, and copy-paste to silently mangle (the same class of risk as the
// BOM-literal issue elsewhere in this codebase) — staticcheck flags this
// (ST1018) for exactly that reason.
const (
	rlm = "\u200f" // RLM (right-to-left mark)
	lrm = "\u200e" // LRM (left-to-right mark)
)

// ShareURL builds a t.me/share/url deep link that opens Telegram's native
// "forward to..." chat picker with text pre-filled. Unlike a
// SwitchInlineQueryChosenChat button, this doesn't require the bot to
// support inline mode and doesn't prefix the target chat's compose box with
// "@BotUsername " — without an inline_query handler, that prefix is never
// resolved into a clean result, so hitting send posts the raw "@BotUsername
// <text>" literally, stepping on whatever was being shared.
func ShareURL(text string) string {
	return "https://t.me/share/url?text=" + url.QueryEscape(text)
}

// ShareURLWithLink is ShareURL plus a url= param, e.g. an invite link back
// to the bot alongside the promo text explaining it.
func ShareURLWithLink(link, text string) string {
	return "https://t.me/share/url?url=" + url.QueryEscape(link) + "&text=" + url.QueryEscape(text)
}

// FormatDate renders d per the user's chosen dateFormat setting.
func FormatDate(d time.Time, dateFormat string) string {
	switch dateFormat {
	case DateFormatDMYDot:
		return fmt.Sprintf("%02d.%02d.%04d", d.Day(), d.Month(), d.Year())
	case DateFormatISO:
		return fmt.Sprintf("%04d-%02d-%02d", d.Year(), d.Month(), d.Day())
	default: // DD/MM/YYYY
		return fmt.Sprintf("%02d/%02d/%04d", d.Day(), d.Month(), d.Year())
	}
}

// FormatTime renders a time of day as HH:MM (24h) or H:MM AM/PM (12h).
func FormatTime(hour, minute int, timeFormat string) string {
	if timeFormat == TimeFormat12h {
		period := "AM"
		if hour >= 12 {
			period = "PM"
		}
		displayHour := hour % 12
		if displayHour == 0 {
			displayHour = 12
		}
		return fmt.Sprintf("%d:%02d %s", displayHour, minute, period)
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// FormatCountdown renders a countdown in Hebrew: "היום" / "מחר" / "עוד יומיים" / "עוד 5 ימים" / ...
func FormatCountdown(days int) string {
	switch {
	case days == 0:
		return "היום"
	case days == 1:
		return "מחר"
	case days < 7:
		return "עוד " + PluralizeHebrew(days, "day")
	case days < 14:
		return "עוד שבוע"
	case days < 30:
		weeks := days / 7
		return "עוד " + PluralizeHebrew(weeks, "week")
	case days < 60:
		return "עוד חודש"
	case days < 90:
		return "עוד חודשיים"
	default:
		months := days / 30
		return "עוד " + PluralizeHebrew(months, "month")
	}
}

// FormatPhone converts an E.164 or raw Israeli phone number into
// "050-1234567" display form.
func FormatPhone(phone string) string {
	var digitsB strings.Builder
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digitsB.WriteRune(c)
		}
	}
	digits := digitsB.String()

	switch {
	case strings.HasPrefix(digits, "972"):
		digits = "0" + digits[3:]
	case !strings.HasPrefix(digits, "0"):
		digits = "0" + digits
	}

	switch len(digits) {
	case 10:
		return digits[:3] + "-" + digits[3:]
	case 9:
		return digits[:2] + "-" + digits[2:]
	default:
		return digits
	}
}

// FormatName joins first and last name, omitting a missing last name cleanly.
func FormatName(firstName string, lastName *string) string {
	if lastName != nil && *lastName != "" {
		return firstName + " " + *lastName
	}
	return firstName
}

// RTL wraps text with a leading RLM to prevent bidi reordering in RTL context.
func RTL(text string) string {
	return rlm + text
}

// LTR wraps text with LRM on both sides, for phones/usernames/Latin dates
// inside RTL text.
func LTR(text string) string {
	return lrm + text + lrm
}
