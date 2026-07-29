// Package i18n loads the locale catalogs and formats named-placeholder strings.
package i18n

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"birthly/locales"
)

const fallbackLanguage = "he"

var (
	mu       sync.RWMutex
	catalogs map[string]map[string]string
)

// LoadLocales loads every <lang>.json file embedded in locales.FS into
// memory, caching them for T. Safe to call multiple times (e.g. in tests);
// the last call wins.
func LoadLocales() error {
	entries, err := fs.ReadDir(locales.FS, ".")
	if err != nil {
		return err
	}
	loaded := make(map[string]map[string]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		lang := strings.TrimSuffix(name, ".json")
		data, err := fs.ReadFile(locales.FS, name)
		if err != nil {
			return err
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			return err
		}
		loaded[lang] = catalog
	}
	mu.Lock()
	catalogs = loaded
	mu.Unlock()
	return nil
}

func ensureLoaded() {
	mu.RLock()
	empty := len(catalogs) == 0
	mu.RUnlock()
	if empty {
		if err := LoadLocales(); err != nil {
			slog.Error("i18n: failed to load locales", "error", err)
		}
	}
}

// T translates key into lang, falling back to Hebrew, then to the key
// itself. If kwargs contains "gender" set to "m" or "f", the "<key>_m" /
// "<key>_f" variant is preferred over the bare key, checked in that gendered
// chain both for lang and for the Hebrew fallback (matches app/i18n/translator.py).
func T(key, lang string, kwargs map[string]any) string {
	ensureLoaded()
	mu.RLock()
	defer mu.RUnlock()

	catalog := catalogs[lang]
	if catalog == nil {
		catalog = catalogs[fallbackLanguage]
	}
	if catalog == nil {
		catalog = map[string]string{}
	}

	gender, _ := kwargs["gender"].(string)
	var template string
	var found bool

	if gender == "m" || gender == "f" {
		template, found = catalog[key+"_"+gender]
	}
	if !found {
		template, found = catalog[key]
	}

	if !found {
		fallbackCatalog := catalogs[fallbackLanguage]
		if gender == "m" || gender == "f" {
			template, found = fallbackCatalog[key+"_"+gender]
		}
		if !found {
			template, found = fallbackCatalog[key]
		}
	}

	if !found {
		slog.Warn("missing_i18n_key", "key", key, "lang", lang)
		return key
	}

	if len(kwargs) == 0 {
		return template
	}
	return formatNamed(template, kwargs)
}

var placeholderRE = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// formatNamed replaces every {ident} placeholder in template with its value
// from kwargs (stringified the same way Python's str.format would render
// each supported kwarg type), mirroring template.format(**kwargs). A
// placeholder with no matching kwarg is left as-is rather than panicking —
// a deliberate, safer default than Python's KeyError for a single missing
// interpolation in one Telegram message.
func formatNamed(template string, kwargs map[string]any) string {
	return placeholderRE.ReplaceAllStringFunc(template, func(match string) string {
		key := match[1 : len(match)-1]
		v, ok := kwargs[key]
		if !ok {
			return match
		}
		return stringifyKwarg(v)
	})
}

func stringifyKwarg(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	default:
		return toStringFallback(v)
	}
}

func toStringFallback(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}
