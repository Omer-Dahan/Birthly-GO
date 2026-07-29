package core

import (
	"fmt"
	"time"

	"github.com/hebcal/hdate"
)

var (
	gematriaOnes     = map[int]string{1: "א", 2: "ב", 3: "ג", 4: "ד", 5: "ה", 6: "ו", 7: "ז", 8: "ח", 9: "ט"}
	gematriaTens     = map[int]string{10: "י", 20: "כ", 30: "ל", 40: "מ", 50: "נ", 60: "ס", 70: "ע", 80: "פ", 90: "צ"}
	gematriaHundreds = map[int]string{100: "ק", 200: "ר", 300: "ש", 400: "ת"}
)

// IsLeapYear is true if hYear (Hebrew year) has 13 months (Adar I + Adar II).
func IsLeapYear(hYear int) bool {
	return hdate.IsLeapYear(hYear)
}

// MonthLength returns the number of days in hMonth of hYear. hMonth uses the
// SPEC.md numbering: 1=Nisan .. 6=Elul, 7=Tishrei .. 11=Shevat, 12=Adar (or
// Adar I in a leap year), 13=Adar II (leap only).
func MonthLength(hYear, hMonth int) (int, error) {
	if hMonth < 1 || hMonth > 13 || (hMonth == 13 && !IsLeapYear(hYear)) {
		return 0, fmt.Errorf("month %d does not exist in Hebrew year %d", hMonth, hYear)
	}
	return hdate.DaysInMonth(hdate.HMonth(hMonth), hYear), nil
}

// ResolveMonth resolves a month number recorded against one leap-status into
// the equivalent month for hYear, applying the Adar policy.
//
//   - Plain months (1-11) pass through unchanged.
//   - srcMonth == 12 (Adar, or Adar I) in a target leap year resolves to 13
//     (Adar II) if policy == "adar_ii", else stays at 12 (Adar I).
//   - srcMonth == 13 (Adar II) in a target non-leap year resolves to 12 (the
//     single Adar).
func ResolveMonth(hYear, srcMonth int, policy string) int {
	targetIsLeap := IsLeapYear(hYear)

	if srcMonth != 12 && srcMonth != 13 {
		return srcMonth
	}

	if targetIsLeap {
		if srcMonth == 13 {
			return 13
		}
		if policy == AdarPolicyAdarII {
			return 13
		}
		return 12
	}

	return 12
}

// ToGregorian converts a Hebrew date to its Gregorian equivalent. If hDay
// does not exist in hMonth of hYear (e.g. the 30th of a 29-day month),
// clamps to the last existing day of that month. Never rolls over into the
// next month. Returns an error if hMonth does not exist in hYear (e.g. Adar
// II requested in a non-leap year) — callers must resolve the month via
// ResolveMonth first when the source leap-status isn't already known-valid.
func ToGregorian(hYear, hMonth, hDay int) (time.Time, error) {
	maxDay, err := MonthLength(hYear, hMonth)
	if err != nil {
		return time.Time{}, err
	}
	day := hDay
	if day > maxDay {
		day = maxDay
	}
	hd := hdate.New(hYear, hdate.HMonth(hMonth), day)
	y, m, d := hd.Greg()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}

// ToHebrew converts a Gregorian date to (hebrew_year, hebrew_month, hebrew_day).
func ToHebrew(d time.Time) (hYear, hMonth, hDay int) {
	hd := hdate.FromGregorian(d.Year(), d.Month(), d.Day())
	return hd.Year(), int(hd.Month()), hd.Day()
}

// HebrewYearGematria renders a Hebrew year as gematria, e.g. 5786 -> "תשפ״ו".
// Drops the leading thousands digit (5), per convention (5786 -> 786).
func HebrewYearGematria(y int) string {
	return Gematria(y % 1000)
}

// Gematria renders an integer (1-999) as a Hebrew gematria string with
// gershayim. Applies the traditional exception: 15 -> ט״ו and 16 -> ט״ז
// instead of the letter combinations that would spell divine names (י״ה / י״ו).
func Gematria(num int) string {
	if num <= 0 {
		return ""
	}

	hundreds, remainder := num/100, num%100
	tens, ones := remainder/10, remainder%10

	letters := ""
	for hundreds > 0 {
		chunk := hundreds
		if chunk > 4 {
			chunk = 4
		}
		letters += gematriaHundreds[chunk*100]
		hundreds -= chunk
	}

	switch remainder {
	case 15:
		letters += "טו"
	case 16:
		letters += "טז"
	default:
		if tens > 0 {
			letters += gematriaTens[tens*10]
		}
		if ones > 0 {
			letters += gematriaOnes[ones]
		}
	}

	runes := []rune(letters)
	if len(runes) == 1 {
		return string(runes) + "׳"
	}
	return string(runes[:len(runes)-1]) + "״" + string(runes[len(runes)-1:])
}

// HebrewMonthName returns the Hebrew name for a SPEC-numbered month (1-13).
func HebrewMonthName(hMonth int) string {
	return HebrewMonthNames[hMonth-1]
}

// FormatHebrewDate formats a Hebrew date as e.g. "י״ד בניסן תשפ״ו" (day,
// month, optional year).
func FormatHebrewDate(hYear, hMonth, hDay int, withYear bool) string {
	dayStr := Gematria(hDay)
	monthName := HebrewMonthName(hMonth)
	result := dayStr + " ב" + monthName
	if withYear {
		result += " " + HebrewYearGematria(hYear)
	}
	return result
}
