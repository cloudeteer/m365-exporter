package intune

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchesProfileFilter(t *testing.T) {
	tests := []struct {
		name        string
		filter      []string
		profileName string
		expected    bool
	}{
		{
			name:        "empty filter matches all",
			filter:      []string{},
			profileName: "Windows Policy",
			expected:    true,
		},
		{
			name:        "exact match lowercase",
			filter:      []string{"windows policy"},
			profileName: "Windows Policy",
			expected:    true,
		},
		{
			name:        "glob pattern match",
			filter:      []string{"windows*"},
			profileName: "Windows Configuration Profile",
			expected:    true,
		},
		{
			name:        "no match",
			filter:      []string{"ios*", "macos*"},
			profileName: "Windows Policy",
			expected:    false,
		},
		{
			name:        "multiple patterns OR logic",
			filter:      []string{"windows*", "apple*"},
			profileName: "Apple Root CA",
			expected:    true,
		},
		{
			name:        "case insensitive",
			filter:      []string{"WINDOWS*"},
			profileName: "windows policy",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Collector{
				profileFilter: toLowerSlice(tt.filter),
			}
			result := c.matchesProfileFilter(tt.profileName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToLowerSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "mixed case",
			input:    []string{"Windows*", "MACOS*", "iOS*"},
			expected: []string{"windows*", "macos*", "ios*"},
		},
		{
			name:     "already lowercase",
			input:    []string{"test", "pattern"},
			expected: []string{"test", "pattern"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toLowerSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeviceConfigurationStatusOverview_StructFields(t *testing.T) {
	// Test that the overview struct has the expected fields and values
	overview := &deviceConfigurationStatusOverview{
		SuccessCount:       29,
		FailedCount:        2,
		ErrorCount:         1,
		PendingCount:       3,
		NotApplicableCount: 5,
	}

	// Verify all 5 status count fields are accessible
	assert.Equal(t, 29, overview.SuccessCount)
	assert.Equal(t, 2, overview.FailedCount)
	assert.Equal(t, 1, overview.ErrorCount)
	assert.Equal(t, 3, overview.PendingCount)
	assert.Equal(t, 5, overview.NotApplicableCount)
}

func TestConfigValidation_InvalidGlobPattern(t *testing.T) {
	// This tests that invalid glob patterns are handled (via path.Match error)
	// The actual validation happens in NewCollector, but we can test the pattern
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	invalidPatterns := []string{
		"windows[",    // unclosed bracket
		"test[a-",     // incomplete range
		"bad\\",       // trailing backslash
	}

	for _, pattern := range invalidPatterns {
		c := &Collector{
			logger:        logger,
			profileFilter: []string{pattern},
		}

		// matchesProfileFilter should handle errors gracefully and skip bad patterns
		// If all patterns are invalid, should return false
		result := c.matchesProfileFilter("Windows Policy")

		// Invalid patterns are skipped, so with only invalid patterns, nothing matches
		assert.False(t, result, "invalid pattern should not match anything")
	}
}

func TestConfigValidation_ValidGlobPatterns(t *testing.T) {
	validPatterns := []string{
		"windows*",
		"*policy",
		"test-[0-9]",
		"app?config",
	}

	for _, pattern := range validPatterns {
		c := &Collector{
			profileFilter: []string{pattern},
		}

		// Just verify that valid patterns don't cause panics
		// Actual matching is tested in TestMatchesProfileFilter
		require.NotPanics(t, func() {
			c.matchesProfileFilter("test string")
		})
	}
}
