package store

import (
	"reflect"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil slice returns empty non-nil slice",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty slice returns empty non-nil slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "trims, lowercases, deduplicates, and drops blanks",
			input:    []string{"  Spanish ", "VERB", "spanish", "", "   ", "Grammar", "verb "},
			expected: []string{"spanish", "verb", "grammar"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTags(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("expected %#v, got %#v", tc.expected, got)
			}
		})
	}
}
