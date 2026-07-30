package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ValidationError carries a ready-to-display, user-friendly Hebrew message.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// ParsedDate is the result of ParseGregorian.
type ParsedDate struct {
	Day   int
	Month int
	Year  *int // nil if no year was present in the input
}

var hebrewMonthAliases = map[string]int{
	"ינואר": 1, "פברואר": 2, "מרץ": 3, "אפריל": 4, "מאי": 5, "יוני": 6,
	"יולי": 7, "אוגוסט": 8, "ספטמבר": 9, "אוקטובר": 10, "נובמבר": 11, "דצמבר": 12,
}

var englishMonthAliases = map[string]int{
	"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3,
	"april": 4, "apr": 4, "may": 5, "june": 6, "jun": 6, "july": 7, "jul": 7,
	"august": 8, "aug": 8, "september": 9, "sep": 9, "sept": 9,
	"october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12,
}

var monthDays = [12]int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// MonthNumberFromName resolves a Hebrew or English month name/abbreviation
// to 1-12, or (0, false).
func MonthNumberFromName(text string) (int, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(text))
	if cleaned == "" {
		return 0, false
	}
	if m, ok := hebrewMonthAliases[strings.TrimSpace(text)]; ok {
		return m, true
	}
	m, ok := englishMonthAliases[cleaned]
	return m, ok
}

var controlCharRE = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

func isValidDayMonth(day, month int) bool {
	if month < 1 || month > 12 {
		return false
	}
	return day >= 1 && day <= monthDays[month-1]
}

func normalizeTwoDigitYear(yy int, now time.Time) int {
	currentYY := now.Year() % 100
	if yy <= currentYY {
		return 2000 + yy
	}
	return 1900 + yy
}

var (
	reDMYFull     = regexp.MustCompile(`^(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{4})$`)
	reDMYShort    = regexp.MustCompile(`^(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{2})$`)
	reISO         = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	reDMNoYear    = regexp.MustCompile(`^(\d{1,2})[/.\-](\d{1,2})$`)
	reHebrewName  = regexp.MustCompile(`^(\d{1,2})\s+ב?([א-ת]+)$`)
	reEnglishName = regexp.MustCompile(`^(\d{1,2})\s+([A-Za-z]+)$`)
)

const badDateMsg = "❌ לא הצלחתי לקרוא את התאריך. נסה בפורמט 15/03/1990"

// ParseGregorian parses free-text Gregorian date input per SPEC.md 15/S3's
// fallback order. now is the reference "current time" used to resolve
// two-digit years (production callers pass time.Now(); tests pass a fixed
// instant for determinism, matching Python's date.today() dependency).
func ParseGregorian(text string, now time.Time) (ParsedDate, error) {
	raw := strings.TrimSpace(text)

	// 1. DD/MM/YYYY, DD.MM.YYYY, DD-MM-YYYY
	if m := reDMYFull.FindStringSubmatch(raw); m != nil {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		if isValidDayMonth(day, month) && year >= MinYearGregorian && year <= MaxYearGregorian {
			return ParsedDate{Day: day, Month: month, Year: &year}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	// 2. DD/MM/YY (two-digit year)
	if m := reDMYShort.FindStringSubmatch(raw); m != nil {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		yy, _ := strconv.Atoi(m[3])
		if isValidDayMonth(day, month) {
			year := normalizeTwoDigitYear(yy, now)
			return ParsedDate{Day: day, Month: month, Year: &year}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	// 5. YYYY-MM-DD (ISO) — checked before the bare DD/MM fallback since it
	//    also uses a separator-delimited numeric form.
	if m := reISO.FindStringSubmatch(raw); m != nil {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		if isValidDayMonth(day, month) && year >= MinYearGregorian && year <= MaxYearGregorian {
			return ParsedDate{Day: day, Month: month, Year: &year}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	// 3. DD/MM or DD.MM — no year
	if m := reDMNoYear.FindStringSubmatch(raw); m != nil {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if isValidDayMonth(day, month) {
			return ParsedDate{Day: day, Month: month, Year: nil}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	// 4. "<day> ב<Hebrew month>" / "<day> <English month name>"
	if m := reHebrewName.FindStringSubmatch(raw); m != nil {
		day, _ := strconv.Atoi(m[1])
		if hebrewMonth, ok := hebrewMonthAliases[m[2]]; ok && isValidDayMonth(day, hebrewMonth) {
			return ParsedDate{Day: day, Month: hebrewMonth, Year: nil}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	if m := reEnglishName.FindStringSubmatch(raw); m != nil {
		day, _ := strconv.Atoi(m[1])
		if englishMonth, ok := englishMonthAliases[strings.ToLower(m[2])]; ok && isValidDayMonth(day, englishMonth) {
			return ParsedDate{Day: day, Month: englishMonth, Year: nil}, nil
		}
		return ParsedDate{}, validationErrorf(badDateMsg)
	}

	return ParsedDate{}, validationErrorf(badDateMsg)
}

func stripControlChars(value string) string {
	return controlCharRE.ReplaceAllString(value, "")
}

// isWhitespaceOnly mirrors Python's str.isspace(): true only for a non-empty
// string consisting entirely of whitespace.
func isWhitespaceOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// ValidateName: 1-64 chars, control characters stripped, not empty/whitespace-only.
func ValidateName(value string) (string, error) {
	cleaned := strings.TrimSpace(stripControlChars(value))
	if cleaned == "" || isWhitespaceOnly(cleaned) {
		return "", validationErrorf("מה השם? 🙂")
	}
	if utf8RuneCount(cleaned) > NameMaxLen {
		return "", validationErrorf("זה ארוך מדי · עד %d תווים.", NameMaxLen)
	}
	return cleaned, nil
}

func validateOptionalText(value string, maxLen int) (string, error) {
	cleaned := strings.TrimSpace(stripControlChars(value))
	if utf8RuneCount(cleaned) > maxLen {
		return "", validationErrorf("זה ארוך מדי · עד %d תווים.", maxLen)
	}
	return cleaned, nil
}

func ValidateNickname(value string) (string, error) {
	return validateOptionalText(value, NicknameMaxLen)
}
func ValidateNotes(value string) (string, error) { return validateOptionalText(value, NotesMaxLen) }
func ValidateRelation(value string) (string, error) {
	return validateOptionalText(value, RelationMaxLen)
}

var (
	phoneSeparatorsRE = regexp.MustCompile(`[\s\-().]`)
	phoneShapeRE      = regexp.MustCompile(`^\+?\d{7,15}$`)
)

// ValidatePhone accepts E.164 or Israeli local format; strips separators, checks length.
func ValidatePhone(value string) (string, error) {
	cleaned := strings.TrimSpace(stripControlChars(value))
	if utf8RuneCount(cleaned) > PhoneMaxLen {
		return "", validationErrorf("זה ארוך מדי · עד %d תווים.", PhoneMaxLen)
	}
	digitsAndPlus := phoneSeparatorsRE.ReplaceAllString(cleaned, "")
	if !phoneShapeRE.MatchString(digitsAndPlus) {
		return "", validationErrorf("❌ מספר הטלפון לא תקין.")
	}
	return digitsAndPlus, nil
}

// ValidateYear checks year against the Gregorian or Hebrew domain range.
func ValidateYear(year int, hebrew bool) (int, error) {
	if hebrew {
		if year < MinYearHebrew || year > MaxYearHebrew {
			return 0, validationErrorf("❌ שנה לא תקינה.")
		}
		return year, nil
	}
	if year < MinYearGregorian || year > MaxYearGregorian {
		return 0, validationErrorf("❌ שנה לא תקינה.")
	}
	return year, nil
}

// ParseHebrewYearInput parses a year typed during Hebrew-date entry,
// accepting either a Hebrew year (e.g. 5750) or a Gregorian year (e.g.
// 1990), auto-detecting by range.
func ParseHebrewYearInput(raw string) (int, error) {
	cleaned := strings.TrimSpace(stripControlChars(raw))
	if cleaned == "" || !isAllASCIIDigits(cleaned) {
		return 0, validationErrorf("❌ שנה לא תקינה.")
	}

	year, err := strconv.Atoi(cleaned)
	if err != nil {
		return 0, validationErrorf("❌ שנה לא תקינה.")
	}
	if year >= MinYearHebrew && year <= MaxYearHebrew {
		return year, nil
	}
	if year >= MinYearGregorian && year <= MaxYearGregorian {
		hYear, _, _ := ToHebrew(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC))
		return hYear, nil
	}
	return 0, validationErrorf("❌ שנה לא תקינה.")
}

// isAllASCIIDigits mirrors Python's str.isdigit() closely enough for this
// input space (Telegram text entry): true if non-empty and every rune is
// 0-9. (Python's isdigit() also accepts some superscript/other Unicode
// digit forms that never appear in practice here.)
func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ContainsOnlySymbols is true if value has no letters or digits (e.g. only
// emoji/punctuation/whitespace).
func ContainsOnlySymbols(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

func utf8RuneCount(s string) int {
	return len([]rune(s))
}
