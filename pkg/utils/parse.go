package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFileSize parses a human-readable file size string like "100MB" or "10MiB"
// and returns the size in bytes.
func ParseFileSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	// Try longer suffixes first manually
	unitMap := map[string]int64{
		"MIB": 1 << 20,
		"MB":  1 << 20,
		"KIB": 1 << 10,
		"KB":  1 << 10,
		"GIB": 1 << 30,
		"GB":  1 << 30,
		"TIB": 1 << 40,
		"TB":  1 << 40,
		"B":   1,
	}

	for _, suffix := range []string{"TIB", "TB", "GIB", "GB", "MIB", "MB", "KIB", "KB", "B"} {
		if strings.HasSuffix(sizeStr, suffix) {
			numStr := strings.TrimSuffix(sizeStr, suffix)
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number in file size: %w", err)
			}
			return int64(num * float64(unitMap[suffix])), nil
		}
	}

	return 0, fmt.Errorf("unrecognized file size unit in %q", sizeStr)
}
