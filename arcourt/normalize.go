package arcourt

import (
	"net/url"
	"sort"
	"strings"
)

// normalizeCaseNumber trims surrounding whitespace and upper-cases letters for
// matching case numbers regardless of how they were typed.
func normalizeCaseNumber(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizedStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeSourceURL(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeSourceURL canonicalizes selection identity; preserve the original URL
// separately for HTTP requests, whose signed query order may be significant.
func normalizeSourceURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Fragment = ""

	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if port == "" {
		parsed.Host = host
	} else {
		parsed.Host = host + ":" + port
	}

	if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}

	if parsed.RawQuery != "" {
		queryValues, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			return trimmed
		}
		// A direct S3 link identifies an object independently of its expiring
		// authentication. Keep resource selectors such as versionId. This never
		// touches RequestURL, and does not guess at other hosts' query semantics.
		if normalizeFetchHostname(host) == arcourtStorageHost {
			for key := range queryValues {
				switch strings.ToLower(key) {
				case "x-amz-algorithm", "x-amz-credential", "x-amz-date", "x-amz-expires", "x-amz-signedheaders", "x-amz-signature", "x-amz-security-token", "awsaccesskeyid", "signature", "expires":
					delete(queryValues, key)
				}
			}
		}
		keys := make([]string, 0, len(queryValues))
		for key := range queryValues {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		normalizedQuery := make(url.Values, len(queryValues))
		for _, key := range keys {
			values := append([]string(nil), queryValues[key]...)
			sort.Strings(values)
			normalizedQuery[key] = values
		}
		parsed.RawQuery = normalizedQuery.Encode()
	}

	return parsed.String()
}
