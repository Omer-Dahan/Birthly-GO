package services

import (
	"context"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"

	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

const userTemplateLimit = core.MaxGreetingTemplatesPerUser

var sentenceWithAgeRE = regexp.MustCompile(`[^.!?。]*\{age\}[^.!?。]*[.!?。]?`)

// PickTemplate returns a randomly-chosen template for (event_type, tone,
// gender, language). When excludeID is non-nil the previously shown template
// is avoided so the same text never appears twice in a row. Returns
// (nil, nil) if no template matches at all (should not happen in a
// fully-seeded DB).
func PickTemplate(ctx context.Context, db repo.DBTX, user *models.User, event *models.Event, tone string, excludeID *int64) (*models.GreetingTemplate, error) {
	templates := repo.NewTemplateRepo(db, user.ID)
	candidates, err := templates.ListMatching(ctx, event.EventType, tone, event.Gender, user.Language)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	if excludeID != nil && len(candidates) > 1 {
		filtered := make([]*models.GreetingTemplate, 0, len(candidates))
		for _, c := range candidates {
			if c.ID != *excludeID {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}

	return candidates[rand.IntN(len(candidates))], nil
}

// RenderTemplate replaces placeholders in tpl.Body with event-specific
// values. Missing values are handled gracefully:
//   - {age} missing  -> entire sentence containing the placeholder is removed.
//   - {nickname}     -> falls back to first_name.
//   - {relation}     -> omitted (replaced with empty string).
func RenderTemplate(tpl *models.GreetingTemplate, event *models.Event, user *models.User) string {
	today := UserToday(user)
	age, hasAge := core.AgeAt(event.CalendarType, event.Year, today)

	body := tpl.Body

	if hasAge {
		body = strings.ReplaceAll(body, "{age}", strconv.Itoa(age))
	} else {
		body = strings.TrimSpace(sentenceWithAgeRE.ReplaceAllString(body, ""))
	}

	body = strings.ReplaceAll(body, "{name}", core.FormatName(event.FirstName, event.LastName))

	nickname := event.FirstName
	if event.Nickname != nil && *event.Nickname != "" {
		nickname = *event.Nickname
	}
	body = strings.ReplaceAll(body, "{nickname}", nickname)

	relation := ""
	if event.Relation != nil {
		relation = *event.Relation
	}
	body = strings.ReplaceAll(body, "{relation}", relation)

	return strings.TrimSpace(body)
}

// CreateUserTemplate creates a personal template for the user (max 20, SPEC.md §22).
func CreateUserTemplate(ctx context.Context, db repo.DBTX, user *models.User, eventType, tone string, gender *string, body string) (*models.GreetingTemplate, error) {
	templates := repo.NewTemplateRepo(db, user.ID)
	count, err := templates.CountUserTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if count >= userTemplateLimit {
		return nil, &LimitErr{Message: "user template limit reached"}
	}

	tpl := &models.GreetingTemplate{
		EventType: eventType,
		Tone:      tone,
		Gender:    gender,
		Language:  user.Language,
		Body:      body,
		IsActive:  true,
	}
	return templates.Create(ctx, tpl)
}

// DeleteUserTemplate soft-deletes a personal template owned by this user.
func DeleteUserTemplate(ctx context.Context, db repo.DBTX, user *models.User, templateID int64) error {
	templates := repo.NewTemplateRepo(db, user.ID)
	tpl, err := templates.GetOwned(ctx, templateID)
	if err != nil {
		return err
	}
	if tpl == nil || tpl.UserID == nil {
		return &NotFoundErr{Message: "template not found for user"}
	}
	return templates.Delete(ctx, templateID)
}

// BuildAIPrompt builds a copy-paste prompt the user hands to their own AI
// assistant, instead of picking a pre-written template (SPEC.md §22 / S14
// "AI" option). The bot never calls an AI itself; it only prepares text for
// the user to paste elsewhere.
func BuildAIPrompt(event *models.Event, user *models.User, tone string) string {
	lang := user.Language
	name := core.FormatName(event.FirstName, event.LastName)
	typeLabel := EventTypeLabel(event.EventType, lang)
	toneLabel := i18n.T("greeting.tone."+tone, lang, nil)

	today := UserToday(user)
	age, hasAge := core.AgeAt(event.CalendarType, event.Year, today)

	details := []string{
		"שם: " + name,
		"סוג אירוע: " + typeLabel,
		"טון מבוקש: " + toneLabel,
	}
	if hasAge {
		details = append(details, "גיל/שנים: "+strconv.Itoa(age))
	}
	if event.Relation != nil && *event.Relation != "" {
		details = append(details, "קשר: "+*event.Relation)
	}
	if event.Nickname != nil && *event.Nickname != "" {
		details = append(details, "כינוי: "+*event.Nickname)
	}
	if event.Notes != nil && *event.Notes != "" {
		details = append(details, "הערות: "+*event.Notes)
	}

	var detailLines []string
	for _, d := range details {
		detailLines = append(detailLines, "- "+d)
	}
	detailsBlock := strings.Join(detailLines, "\n")

	return "כתוב עבורי ברכה קצרה וחמה בעברית, על סמך הפרטים הבאים:\n\n" +
		detailsBlock + "\n\n" +
		"אנא תכתוב ברכה מוכנה, בלי הסברים נוספים."
}
