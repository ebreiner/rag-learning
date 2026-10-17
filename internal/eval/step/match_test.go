package step

import "testing"

func TestMatches(t *testing.T) {
	chunk := Chunk{
		DocName:    "a.md",
		Breadcrumb: "Setup > Install",
		Text:       "Run the Installer\nwith admin rights.",
	}

	tests := []struct {
		name string
		e    Expectation
		want bool
	}{
		{"different doc", Expectation{Doc: "b.md"}, false},
		{"doc only", Expectation{Doc: "a.md"}, true},
		{"breadcrumb matches", Expectation{Doc: "a.md", BreadcrumbContains: "install"}, true},
		{"breadcrumb does not match", Expectation{Doc: "a.md", BreadcrumbContains: "Usage"}, false},
		{"text matches", Expectation{Doc: "a.md", TextContains: "admin rights"}, true},
		{"text does not match", Expectation{Doc: "a.md", TextContains: "root rights"}, false},
		{"both set, one false", Expectation{Doc: "a.md", BreadcrumbContains: "Install", TextContains: "root"}, false},
		{"both set, both true", Expectation{Doc: "a.md", BreadcrumbContains: "Install", TextContains: "installer"}, true},
		{"newline in text, space in anchor", Expectation{Doc: "a.md", TextContains: "installer with admin"}, true},
		{"different case", Expectation{Doc: "a.md", TextContains: "RUN THE INSTALLER"}, true},
		{"extra spaces in anchor", Expectation{Doc: "a.md", TextContains: "  the   installer "}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matches(tt.e, chunk); got != tt.want {
				t.Errorf("matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
