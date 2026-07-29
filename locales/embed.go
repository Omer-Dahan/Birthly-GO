// Package locales embeds the <lang>.json translation catalogs.
package locales

import "embed"

//go:embed *.json
var FS embed.FS
