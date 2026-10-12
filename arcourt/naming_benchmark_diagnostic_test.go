package arcourt

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// benchmarkDiagnosticClient observes only synthetic benchmark responses. The
// report retains controlled categories, never the rejected provider text.
type benchmarkDiagnosticClient struct {
	namingClient
	observe func(raw, excerpt string)
}

func (c benchmarkDiagnosticClient) Generate(ctx context.Context, prompt, excerpt string) (string, NamingUsage, error) {
	raw, usage, err := c.namingClient.Generate(ctx, prompt, excerpt)
	if err == nil {
		c.observe(raw, excerpt)
	}
	return raw, usage, err
}

func namingBenchmarkOutputDiagnostic(raw, excerpt string) string {
	_, reason := normalizeNamingLabel(raw, excerpt)
	switch reason {
	case "":
		return "accepted"
	case "insufficient_context":
		return "abstain"
	case "unsafe_result":
		label := strings.TrimSpace(raw)
		switch {
		case label == "":
			return "unsafe_empty"
		case len(label) > maxNamingLabelBytes:
			return "unsafe_overlong"
		case !utf8.ValidString(label) || !isNamingASCII(label):
			return "unsafe_non_ascii"
		case strings.ContainsAny(label, "\r\n\t"):
			return "unsafe_multiline"
		case strings.ContainsAny(label, ".:/\\"):
			return "unsafe_path_or_sentence_punctuation"
		case strings.Contains(label, "  "):
			return "unsafe_repeated_space"
		default:
			return "unsafe_other_format"
		}
	case "unsupported_result":
		switch {
		case !namingLabelType.MatchString(raw):
			return "unsupported_missing_type"
		case !groundedNamingLabel(raw, excerpt):
			return "unsupported_ungrounded"
		case !preservesNamingHeading(raw, excerpt):
			return "unsupported_heading"
		default:
			return "unsupported_other"
		}
	default:
		return "other_rejection"
	}
}

func isNamingASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func TestNamingBenchmarkOutputDiagnostic(t *testing.T) {
	excerpt := "DEFENDANT'S SECOND AMENDED MOTION TO COMPEL\nDefendant requests discovery."
	for _, tc := range []struct{ raw, want string }{
		{"Defendant Second Amended Motion to Compel", "accepted"},
		{"ABSTAIN", "abstain"},
		{"", "unsafe_empty"},
		{strings.Repeat("Motion ", 12), "unsafe_overlong"},
		{"Mótion to Compel", "unsafe_non_ascii"},
		{"Motion to\nCompel", "unsafe_multiline"},
		{"Motion to Compel.", "unsafe_path_or_sentence_punctuation"},
		{"Motion  to Compel", "unsafe_repeated_space"},
		{"`Motion to Compel`", "unsafe_other_format"},
		{"Generic Document", "unsupported_missing_type"},
		{"Plaintiff Motion to Compel", "unsupported_ungrounded"},
		{"Defendant Second Amended Notice to Compel", "unsupported_ungrounded"},
		{"Defendant Motion to Compel", "unsupported_heading"},
	} {
		if got := namingBenchmarkOutputDiagnostic(tc.raw, excerpt); got != tc.want {
			t.Errorf("diagnostic for %q = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
