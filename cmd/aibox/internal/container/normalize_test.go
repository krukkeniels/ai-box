package container

import "testing"

func TestNormalizeImageRef(t *testing.T) {
	tests := []struct {
		input    string
		hasLocal bool
		want     string
	}{
		{"ghcr.io/org/img:tag", false, "ghcr.io/org/img:tag"},
		{"ghcr.io/org/img:tag", true, "ghcr.io/org/img:tag"},
		{"aibox-base:24.04", true, "localhost/aibox-base:24.04"},
		{"aibox-base:24.04", false, "aibox-base:24.04"},
		{"localhost/img:tag", true, "localhost/img:tag"},
		{"localhost/img:tag", false, "localhost/img:tag"},
		{"docker.io/library/ubuntu:24.04", false, "docker.io/library/ubuntu:24.04"},
		{"myimage", true, "localhost/myimage"},
		{"myimage", false, "myimage"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeImageRef(tt.input, tt.hasLocal)
			if got != tt.want {
				t.Errorf("normalizeImageRef(%q, %v) = %q, want %q", tt.input, tt.hasLocal, got, tt.want)
			}
		})
	}
}
