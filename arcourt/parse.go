package arcourt

import "strings"

type linkRow struct {
	URL         string
	SourceURL   string
	Description string
	FilingDate  string
}

func normalizeLinkRows(rows []linkRow) []linkRow {
	seen := make(map[string]int, len(rows))
	out := make([]linkRow, 0, len(rows))
	for _, row := range rows {
		url := strings.TrimSpace(row.URL)
		if url == "" {
			continue
		}

		sourceURL := strings.TrimSpace(row.SourceURL)
		if sourceURL == "" {
			sourceURL = url
		}
		description := strings.TrimSpace(row.Description)
		filingDate := strings.TrimSpace(row.FilingDate)

		if idx, ok := seen[normalizeSourceURL(sourceURL)]; ok {
			if out[idx].Description == "" && description != "" {
				out[idx].Description = description
			}
			if out[idx].FilingDate == "" && filingDate != "" {
				out[idx].FilingDate = filingDate
			}
			if out[idx].SourceURL == "" {
				out[idx].SourceURL = sourceURL
			}
			continue
		}

		seen[normalizeSourceURL(sourceURL)] = len(out)
		out = append(out, linkRow{
			URL:         url,
			SourceURL:   sourceURL,
			Description: description,
			FilingDate:  filingDate,
		})
	}
	return out
}
