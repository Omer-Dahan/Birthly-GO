// Package version carries the build stamp: which version, commit and build
// date a binary was made from. The values are injected at link time:
//
//	-ldflags "-X birthly/internal/version.Version=... \
//	          -X birthly/internal/version.Commit=...  \
//	          -X birthly/internal/version.Date=..."
//
// A plain `go build` leaves them unset and every field reports "dev".
package version

// Product is the product name shown next to the stamp.
const Product = "Birthly"

// Fallback is what an unstamped (or empty) field reports.
const Fallback = "dev"

// Set by the linker; do not assign elsewhere.
var (
	Version string
	Commit  string
	Date    string
)

// Info is the resolved stamp, never containing an empty value.
type Info struct {
	Version string
	Commit  string
	Date    string
}

func orDev(s string) string {
	if s == "" {
		return Fallback
	}
	return s
}

// Get returns the stamp with "dev" substituted for any missing value.
func Get() Info {
	return Info{Version: orDev(Version), Commit: orDev(Commit), Date: orDev(Date)}
}
