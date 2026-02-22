package container

import "testing"

func TestParseMemoryGB(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"4g", 4},
		{"8g", 8},
		{"16g", 16},
		{"8G", 8},
		{"8Gi", 8},
		{"8gb", 8},
		{"512m", 0},    // 512 MB < 1 GB
		{"2048m", 2},   // 2048 MB = 2 GB
		{"", 0},        // empty
		{"invalid", 0}, // invalid
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMemoryGB(tt.input)
			if got != tt.want {
				t.Errorf("parseMemoryGB(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
