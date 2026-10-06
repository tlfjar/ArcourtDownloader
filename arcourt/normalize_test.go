package arcourt

import "testing"

func TestNormalizeCaseNumber(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"  60cv-2026-1  ", "60CV-2026-1"},
		{"60cv-1", "60CV-1"},
		{"60CV-1", "60CV-1"},
		{"\t60dr-24-42\n", "60DR-24-42"},
		{"", ""},
		{" \t\n", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeCaseNumber(tc.input); got != tc.want {
				t.Fatalf("normalizeCaseNumber(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeSourceURL(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{" HTTPS://EXAMPLE.COM:443/doc/ABC/?b=2&a=3&a=1#ignored ", "https://example.com/doc/ABC?a=1&a=3&b=2"},
		{"http://EXAMPLE.COM:80/", "http://example.com/"},
		{"https://EXAMPLE.COM:8443/doc/", "https://example.com:8443/doc"},
		{" /relative/doc ", "/relative/doc"},
		{" :invalid ", ":invalid"},
		{" \t", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeSourceURL(tc.input); got != tc.want {
				t.Fatalf("normalizeSourceURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizedStringSliceEmpty(t *testing.T) {
	for _, values := range [][]string{nil, {}, {"", " \t"}} {
		if got := normalizedStringSlice(values); got != nil {
			t.Fatalf("normalizedStringSlice(%q) = %q, want nil", values, got)
		}
	}
}

func TestNormalizedStringSliceCanonicalizesURLs(t *testing.T) {
	got := normalizedStringSlice([]string{
		" https://example.com/opad/api/documents/ABC?b=2&a=1#frag ",
		"https://EXAMPLE.com/opad/api/documents/ABC?a=1&b=2",
		"",
	})

	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0] != "https://example.com/opad/api/documents/ABC?a=1&b=2" {
		t.Fatalf("canonical url=%q, want %q", got[0], "https://example.com/opad/api/documents/ABC?a=1&b=2")
	}
}

func TestStorageIdentityExcludesExpiringAuthentication(t *testing.T) {
	// Deliberately invalid authentication values; never a usable signed URL.
	base := "https://" + arcourtStorageHost + "/fixture.pdf"
	first := base + "?versionId=fixture&X-Amz-Signature=invalid-one&X-Amz-Date=invalid-date&X-Amz-Security-Token=invalid-token"
	second := base + "?X-Amz-Signature=invalid-two&versionId=fixture&X-Amz-Expires=invalid-expiry"
	if normalizeSourceURL(first) != base+"?versionId=fixture" || normalizeSourceURL(first) != normalizeSourceURL(second) {
		t.Fatal("expiring authentication changed storage identity")
	}
	rows := normalizeLinkRows([]linkRow{{URL: first}, {URL: second}})
	if len(rows) != 1 || rows[0].URL != first {
		t.Fatal("identity normalization changed the request URL")
	}
	if normalizeSourceURL(base+"?versionId=other") == normalizeSourceURL(first) {
		t.Fatal("resource version was removed")
	}
	malformed := "https://arcourts.gov/document?identity=%zz"
	if normalizeSourceURL(malformed) != malformed {
		t.Fatal("malformed query was silently discarded")
	}
}
