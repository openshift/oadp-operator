package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFileSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1MB", 1 << 20},
		{"10MiB", 10 << 20},
		{"100KB", 100 << 10},
		{"1GiB", 1 << 30},
		{"1B", 1},
	}

	for _, tt := range tests {
		got, err := ParseFileSize(tt.input)
		require.NoError(t, err, "input: %s", tt.input)
		require.Equal(t, tt.expected, got, "input: %s", tt.input)
	}
}
