package arcourt

import (
	"regexp"
	"strings"
)

var invalidFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// SanitizeFilename returns a portable PDF basename of at most 120 ASCII bytes.
// It does not establish uniqueness; the download service adds document identity
// and checks directory entries case-insensitively before publishing.
func SanitizeFilename(description string, fallback string) string {
	name := strings.TrimSpace(description)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		name = "document"
	}
	name = strings.ToLower(name)
	name = strings.TrimRight(name, ". ")
	name = strings.TrimSuffix(name, ".pdf")
	name = strings.ReplaceAll(name, "&", " and ")
	name = invalidFilenameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._-")
	if name == "" {
		name = "document"
	}
	stem := strings.SplitN(name, ".", 2)[0]
	if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || stem == "clock$" ||
		(len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '1' && stem[3] <= '9') {
		name = "_" + name
	}
	if len(name) > 116 {
		name = strings.TrimRight(name[:116], "._-")
	}
	return name + ".pdf"
}
