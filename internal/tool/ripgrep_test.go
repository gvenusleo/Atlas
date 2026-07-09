package tool

import "testing"

func TestRgGlobPattern(t *testing.T) {
	tests := []struct{ in, want string }{
		{"*.go", "/*.go"},
		{"**/*.go", "/**/*.go"},
		{"config/*.js", "/config/*.js"},
		{"/config/*.js", "/config/*.js"},
		{"**/config/*.{js,json}", "/**/config/*.{js,json}"},
	}
	for _, tt := range tests {
		if got := rgGlobPattern(tt.in); got != tt.want {
			t.Errorf("rgGlobPattern(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
