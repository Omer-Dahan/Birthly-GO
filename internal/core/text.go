package core

import (
	"strconv"
	"strings"
)

var dualForms = map[string]string{
	"day": "יומיים", "year": "שנתיים", "week": "שבועיים", "month": "חודשיים", "person": "שני אנשים",
}

var singularForms = map[string]string{
	"day": "יום", "year": "שנה", "week": "שבוע", "month": "חודש", "person": "אדם",
}

var pluralForms = map[string]string{
	"day": "ימים", "year": "שנים", "week": "שבועות", "month": "חודשים", "person": "אנשים",
}

// Esc HTML-escapes user-supplied text before embedding in an HTML-parse-mode
// message. Matches Python's html.escape(value, quote=False): escapes
// &, <, > only — quotes are left alone.
func Esc(value string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(value)
}

// PluralizeHebrew renders count of unit ("day", "year", "week", "person") in
// Hebrew. Handles the dual form (2 -> "יומיים" not "2 ימים") and the
// standard singular ("1 יום") / plural ("5 ימים") forms. Panics for an
// unknown unit, matching Python's ValueError (a programmer error, not
// user input — every call site passes a literal).
func PluralizeHebrew(count int, unit string) string {
	singular, ok := singularForms[unit]
	if !ok {
		panic("unknown unit: " + unit)
	}
	if count == 1 {
		return "1 " + singular
	}
	if count == 2 {
		return dualForms[unit]
	}
	return strconv.Itoa(count) + " " + pluralForms[unit]
}

// Truncate shortens value to at most maxLen characters (Unicode codepoints),
// appending suffix if cut.
func Truncate(value string, maxLen int, suffix string) string {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	suffixRunes := []rune(suffix)
	if maxLen <= len(suffixRunes) {
		if maxLen >= len(suffixRunes) {
			return suffix
		}
		return string(suffixRunes[:maxLen])
	}
	return string(runes[:maxLen-len(suffixRunes)]) + suffix
}

// SplitName splits "Dana Cohen" into ("Dana", "Cohen") on the first space.
// A name with no space becomes (name, nil) — no last name.
func SplitName(fullName string) (string, *string) {
	first, rest, found := strings.Cut(fullName, " ")
	if !found {
		return first, nil
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return first, nil
	}
	return first, &rest
}
