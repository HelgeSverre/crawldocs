package main

import (
	"strings"
	"testing"
)

func TestCleanHTMLSimple(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "CSS font-face removal",
			input:    "Documentation @font-face { font-family: 'Be Vietnam Pro'; src: url(...); } Some content here",
			expected: "Documentation Some content here",
		},
		{
			name:     "Excessive whitespace",
			input:    "Title   \n\n\n    Content    with   spaces",
			expected: "Title Content with spaces",
		},
		{
			name:     "CSS property removal",
			input:    "Content with color: red; and font-size: 14px; properties",
			expected: "Content with and properties",
		},
		{
			name:     "URL function removal",
			input:    "Background url(image.jpg) and format(woff2) functions",
			expected: "Background and functions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanHTMLSimple(tt.input)
			t.Logf("Input: %q", tt.input)
			t.Logf("Result: %q", result)
			t.Logf("Expected: %q", tt.expected)
			if !strings.Contains(result, strings.TrimSpace(tt.expected)) {
				t.Errorf("cleanHTML() = %q, want to contain %q", result, tt.expected)
			}
		})
	}
}
