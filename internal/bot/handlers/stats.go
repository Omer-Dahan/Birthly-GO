package handlers

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
)

// RegisterStats wires the S13 stats screen — port of app/handlers/stats.py.
func RegisterStats(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("stat"), cbMenuStats))
	dispatcher.AddHandler(handlers.NewCallback(statsActionFilter("home"), cbStatsHome))
	dispatcher.AddHandler(handlers.NewCallback(statsDrilldownFilter(), cbStatsDrilldown))
}

func statsActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixStats {
			return false
		}
		s, err := callbacks.DecodeStats(cq.Data)
		return err == nil && s.Action == action
	}
}

func statsDrilldownFilter() filters.CallbackQuery {
	drilldown := map[string]bool{"months": true, "cats": true, "ages": true}
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixStats {
			return false
		}
		s, err := callbacks.DecodeStats(cq.Data)
		return err == nil && drilldown[s.Action]
	}
}

var monthNamesEN = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

const barMaxLen = 10

func statsBar(count, maxCount int) string {
	if maxCount <= 0 {
		return ""
	}
	filled := 0
	if count > 0 {
		filled = int(math.Round(float64(count) / float64(maxCount) * barMaxLen))
		if filled < 1 {
			filled = 1
		}
	}
	return strings.Repeat("▓", filled)
}

func monthLabel(month int, lang string) string {
	if lang == core.LanguageHe {
		if month >= 1 && month <= 12 {
			return core.HebrewMonthNames[month-1]
		}
		return strconv.Itoa(month)
	}
	if month >= 1 && month <= 12 {
		return monthNamesEN[month-1]
	}
	return strconv.Itoa(month)
}

func statsHomeKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	btn := func(action, key string) gotgbot.InlineKeyboardButton {
		return gotgbot.InlineKeyboardButton{Text: i18n.T(key, lang, nil), CallbackData: callbacks.Stats{Action: action}.Encode()}
	}
	months := btn("months", "stats.by_months_btn")
	cats := btn("cats", "stats.by_categories_btn")
	ages := btn("ages", "stats.by_ages_btn")
	home := keyboards.HomeButton(i18n.T("common.home", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{months, cats, ages}, {home}}}
}

func statsDrilldownKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	back := gotgbot.InlineKeyboardButton{Text: i18n.T("common.back", lang, nil), CallbackData: callbacks.Stats{Action: "home"}.Encode()}
	home := keyboards.HomeButton(i18n.T("common.home", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{back, home}}}
}

func renderStatsHome(stats *services.UserStats, user *models.User) string {
	lang := user.Language
	if stats.Total == 0 {
		return i18n.T("stats.title", lang, nil) + "\n\n" + i18n.T("stats.empty", lang, nil)
	}

	lines := []string{
		i18n.T("stats.title", lang, nil), "",
		i18n.T("stats.total", lang, map[string]any{"count": stats.Total}),
		i18n.T("stats.today", lang, map[string]any{"count": stats.Today}),
		i18n.T("stats.this_week", lang, map[string]any{"count": stats.ThisWeek}),
		i18n.T("stats.this_month", lang, map[string]any{"count": stats.ThisMonth}),
		"",
	}

	if stats.Nearest != nil && stats.NearestDaysUntil != nil {
		name := core.Esc(core.FormatName(stats.Nearest.FirstName, stats.Nearest.LastName))
		countdown := core.FormatCountdown(*stats.NearestDaysUntil)
		lines = append(lines, i18n.T("stats.nearest", lang, map[string]any{"name": name, "countdown": countdown}))
	}

	if stats.Youngest != nil {
		lines = append(lines, statsAgeLine("stats.youngest", stats.Youngest, lang))
	}
	if stats.Oldest != nil {
		lines = append(lines, statsAgeLine("stats.oldest", stats.Oldest, lang))
	}
	if stats.AvgAge != nil {
		lines = append(lines, i18n.T("stats.avg_age", lang, map[string]any{"age": int(math.Round(*stats.AvgAge))}))
	}

	if len(stats.ByCategoryOrder) > 0 {
		lines = append(lines, "", i18n.T("stats.by_category_title", lang, nil))
		parts := make([]string, len(stats.ByCategoryOrder))
		for i, cat := range stats.ByCategoryOrder {
			parts[i] = services.CategoryLabel(cat, lang) + " " + strconv.Itoa(stats.ByCategory[cat])
		}
		lines = append(lines, strings.Join(parts, " · "))
	}

	if len(stats.ByMonth) > 0 {
		lines = append(lines, "", i18n.T("stats.by_month_title", lang, nil))
		maxCount := 0
		months := make([]int, 0, len(stats.ByMonth))
		for m, c := range stats.ByMonth {
			months = append(months, m)
			if c > maxCount {
				maxCount = c
			}
		}
		sort.Ints(months)
		for _, m := range months {
			count := stats.ByMonth[m]
			lines = append(lines, monthLabel(m, lang)+" "+statsBar(count, maxCount)+" "+strconv.Itoa(count))
		}
	}

	return strings.Join(lines, "\n")
}

func statsAgeLine(key string, ea *services.EventAge, lang string) string {
	name := core.Esc(core.FormatName(ea.Event.FirstName, ea.Event.LastName))
	kw := map[string]any{"name": name, "age": ea.Age}
	if ea.Event.Gender != nil {
		kw["gender"] = *ea.Event.Gender
	}
	return i18n.T(key, lang, kw)
}

func ageBucketLabel(bucket int) string {
	return strconv.Itoa(bucket) + "-" + strconv.Itoa(bucket+9)
}

func renderStatsMonths(stats *services.UserStats, user *models.User) string {
	lang := user.Language
	if stats.Total == 0 {
		return i18n.T("stats.title", lang, nil) + "\n\n" + i18n.T("stats.empty", lang, nil)
	}
	maxCount := 0
	for _, c := range stats.ByMonth {
		if c > maxCount {
			maxCount = c
		}
	}
	lines := []string{i18n.T("stats.by_month_title", lang, nil), ""}
	for m := 1; m <= 12; m++ {
		count := stats.ByMonth[m]
		lines = append(lines, monthLabel(m, lang)+"  "+statsBar(count, maxCount)+" "+strconv.Itoa(count))
	}
	return strings.Join(lines, "\n")
}

func renderStatsCategories(stats *services.UserStats, user *models.User) string {
	lang := user.Language
	if stats.Total == 0 {
		return i18n.T("stats.title", lang, nil) + "\n\n" + i18n.T("stats.empty", lang, nil)
	}
	maxCount := 0
	for _, cat := range stats.ByCategoryOrder {
		if c := stats.ByCategory[cat]; c > maxCount {
			maxCount = c
		}
	}
	lines := []string{i18n.T("stats.by_category_title", lang, nil), ""}
	for _, cat := range stats.ByCategoryOrder {
		count := stats.ByCategory[cat]
		lines = append(lines, services.CategoryLabel(cat, lang)+"  "+statsBar(count, maxCount)+" "+strconv.Itoa(count))
	}
	return strings.Join(lines, "\n")
}

func renderStatsAges(stats *services.UserStats, user *models.User) string {
	lang := user.Language
	if len(stats.AgeBuckets) == 0 {
		return i18n.T("stats.title", lang, nil) + "\n\n" + i18n.T("stats.empty", lang, nil)
	}

	buckets := make([]int, 0, len(stats.AgeBuckets))
	maxCount := 0
	for bucket, c := range stats.AgeBuckets {
		buckets = append(buckets, bucket)
		if c > maxCount {
			maxCount = c
		}
	}
	sort.Ints(buckets)

	lines := []string{i18n.T("stats.by_age_title", lang, nil), ""}
	for _, bucket := range buckets {
		count := stats.AgeBuckets[bucket]
		lines = append(lines, ageBucketLabel(bucket)+"  "+statsBar(count, maxCount)+" "+strconv.Itoa(count))
	}

	if stats.Youngest != nil || stats.Oldest != nil || stats.AvgAge != nil {
		lines = append(lines, "")
	}
	if stats.Youngest != nil {
		lines = append(lines, statsAgeLine("stats.youngest", stats.Youngest, lang))
	}
	if stats.Oldest != nil {
		lines = append(lines, statsAgeLine("stats.oldest", stats.Oldest, lang))
	}
	if stats.AvgAge != nil {
		lines = append(lines, i18n.T("stats.avg_age", lang, map[string]any{"age": int(math.Round(*stats.AvgAge))}))
	}
	return strings.Join(lines, "\n")
}

func renderStatsAndAnswer(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)

	stats, err := services.GetUserStats(context.Background(), db, user)
	if err != nil {
		return err
	}
	text := renderStatsHome(stats, user)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, statsHomeKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbMenuStats(b *gotgbot.Bot, ctx *ext.Context) error { return renderStatsAndAnswer(b, ctx) }
func cbStatsHome(b *gotgbot.Bot, ctx *ext.Context) error { return renderStatsAndAnswer(b, ctx) }

// cbStatsDrilldown renders the months/cats/ages breakdown the user tapped —
// each is its own focused screen rather than the aggregate home view, since
// re-rendering the same home screen for every button made the drilldown
// buttons look broken (tapping any of the three produced an identical
// screen, no visible change).
func cbStatsDrilldown(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)

	s, err := callbacks.DecodeStats(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}

	stats, err := services.GetUserStats(context.Background(), db, user)
	if err != nil {
		return err
	}

	var text string
	switch s.Action {
	case "months":
		text = renderStatsMonths(stats, user)
	case "cats":
		text = renderStatsCategories(stats, user)
	case "ages":
		text = renderStatsAges(stats, user)
	default:
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	if err := EditOrIgnore(b, ctx.CallbackQuery, text, statsDrilldownKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}
