package services

import (
	"context"
	"strings"

	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

var allCategories = []string{
	core.CategoryFamily, core.CategoryFriends, core.CategoryWork,
	core.CategoryClients, core.CategorySchool, core.CategoryOther,
}

// categoryFromQuery resolves free text to a Category value if it matches a
// category's localized name.
func categoryFromQuery(query, lang string) *string {
	cleaned := strings.TrimSpace(query)
	for _, category := range allCategories {
		if i18n.T(categoryKey(category), lang, nil) == cleaned {
			c := category
			return &c
		}
	}
	return nil
}

// SearchEvents implements S10: resolve the query to a category/month filter
// if it matches one, otherwise fall back to a free-text LIKE search across
// name/nickname/phone/notes/relation.
func SearchEvents(ctx context.Context, db repo.DBTX, user *models.User, query string) ([]*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)

	if category := categoryFromQuery(query, user.Language); category != nil {
		return events.Search(ctx, nil, category, nil)
	}

	if month, ok := core.MonthNumberFromName(query); ok {
		return events.Search(ctx, nil, nil, &month)
	}

	return events.Search(ctx, &query, nil, nil)
}
