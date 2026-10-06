package arcourt

import (
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name        string
		description string
		fallback    string
		want        string
	}{
		{name: "normal text", description: "Summons and Complaint", fallback: "", want: "summons_and_complaint.pdf"},
		{name: "strips symbols", description: "Order: #12 / Entered", fallback: "", want: "order_12_entered.pdf"},
		{name: "uses fallback", description: "", fallback: "Docket Entry", want: "docket_entry.pdf"},
		{name: "empty default", description: "", fallback: "", want: "document.pdf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeFilename(tc.description, tc.fallback)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeFilenameWindowsAndExtensions(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"CON", "_con.pdf"}, {"NuL.PDF", "_nul.pdf"}, {"PRN.txt", "_prn.txt.pdf"},
		{"AUX.", "_aux.pdf"}, {"COM1.pdf", "_com1.pdf"}, {"LPT9", "_lpt9.pdf"},
		{"COM10", "com10.pdf"}, {"COM¹", "com.pdf"}, {"CONIN$", "conin.pdf"},
		{"Report.PDF.  ", "report.pdf"}, {"Order.doc", "order.doc.pdf"},
		{"../../CON.pdf", "_con.pdf"}, {`C:\outside\<bad>:"*?|`, "c_outside_bad.pdf"},
		{". . .", "document.pdf"},
	} {
		if got := SanitizeFilename(tc.input, ""); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.input, got, tc.want)
		}
	}
	name := SanitizeFilename(strings.Repeat("long title ", 100), "")
	if len(name) > 120 || !strings.HasSuffix(name, ".pdf") || !safeComponent(name) {
		t.Fatalf("unsafe bounded component: %q", name)
	}
}
