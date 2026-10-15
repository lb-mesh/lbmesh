package recipe

import "testing"

func TestURLToSlug(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		// HTTPS variants
		{"https://github.com/acme/recipes", "github.com/acme/recipes", false},
		{"https://github.com/acme/recipes.git", "github.com/acme/recipes", false},
		{"https://gitlab.com/team/private-tap.git", "gitlab.com/team/private-tap", false},
		// SSH SCP-style
		{"git@github.com:acme/recipes.git", "github.com/acme/recipes", false},
		{"git@github.com:acme/recipes", "github.com/acme/recipes", false},
		{"git@gitlab.com:team/private-tap.git", "gitlab.com/team/private-tap", false},
		// SSH URL scheme
		{"ssh://git@github.com/acme/recipes.git", "github.com/acme/recipes", false},
		// Invalid
		{"not-a-url", "", true},
	}

	for _, tc := range tests {
		got, err := urlToSlug(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("urlToSlug(%q): expected error, got %q", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("urlToSlug(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("urlToSlug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
