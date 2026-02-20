package doctor

import (
	"encoding/json"
	"testing"
)

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		name      string
		ver       string
		wantMajor int
		wantMinor int
	}{
		{
			name:      "standard WSL2 kernel",
			ver:       "5.15.90.1-microsoft-standard-WSL2",
			wantMajor: 5,
			wantMinor: 15,
		},
		{
			name:      "modern kernel",
			ver:       "6.1.21-generic",
			wantMajor: 6,
			wantMinor: 1,
		},
		{
			name:      "major only with dot",
			ver:       "5.4",
			wantMajor: 5,
			wantMinor: 4,
		},
		{
			name:      "three-part version",
			ver:       "6.5.0",
			wantMajor: 6,
			wantMinor: 5,
		},
		{
			name:      "empty string",
			ver:       "",
			wantMajor: 0,
			wantMinor: 0,
		},
		{
			name:      "no dots",
			ver:       "6",
			wantMajor: 0,
			wantMinor: 0,
		},
		{
			name:      "non-numeric",
			ver:       "abc.def.ghi",
			wantMajor: 0,
			wantMinor: 0,
		},
		{
			name:      "minor with dash suffix",
			ver:       "5.15-custom",
			wantMajor: 5,
			wantMinor: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor := parseKernelVersion(tt.ver)
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseKernelVersion(%q) = (%d, %d), want (%d, %d)",
					tt.ver, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestReport_HasFailures(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    bool
	}{
		{
			name:    "empty report",
			results: nil,
			want:    false,
		},
		{
			name: "all passing",
			results: []CheckResult{
				{Name: "check1", Status: "pass"},
				{Name: "check2", Status: "pass"},
			},
			want: false,
		},
		{
			name: "one failure",
			results: []CheckResult{
				{Name: "check1", Status: "pass"},
				{Name: "check2", Status: "fail"},
			},
			want: true,
		},
		{
			name: "warnings only",
			results: []CheckResult{
				{Name: "check1", Status: "warn"},
				{Name: "check2", Status: "warn"},
			},
			want: false,
		},
		{
			name: "mixed with failure",
			results: []CheckResult{
				{Name: "check1", Status: "pass"},
				{Name: "check2", Status: "warn"},
				{Name: "check3", Status: "fail"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Results: tt.results}
			if got := r.HasFailures(); got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReport_JSON(t *testing.T) {
	r := &Report{
		Results: []CheckResult{
			{
				Name:    "Container Runtime",
				Status:  "pass",
				Message: "podman: 4.9.0",
			},
			{
				Name:        "gVisor Runtime",
				Status:      "fail",
				Message:     "runsc not found",
				Remediation: "Install gVisor",
			},
		},
	}

	out, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	// Verify it's valid JSON.
	var parsed Report
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON() output is not valid JSON: %v", err)
	}

	// Verify the round-tripped data matches.
	if len(parsed.Results) != 2 {
		t.Fatalf("JSON round-trip: got %d results, want 2", len(parsed.Results))
	}
	if parsed.Results[0].Name != "Container Runtime" {
		t.Errorf("Results[0].Name = %q, want %q", parsed.Results[0].Name, "Container Runtime")
	}
	if parsed.Results[0].Status != "pass" {
		t.Errorf("Results[0].Status = %q, want %q", parsed.Results[0].Status, "pass")
	}
	if parsed.Results[1].Status != "fail" {
		t.Errorf("Results[1].Status = %q, want %q", parsed.Results[1].Status, "fail")
	}
	if parsed.Results[1].Remediation != "Install gVisor" {
		t.Errorf("Results[1].Remediation = %q, want %q", parsed.Results[1].Remediation, "Install gVisor")
	}
}

func TestReport_JSON_Empty(t *testing.T) {
	r := &Report{}
	out, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	var parsed Report
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON() output is not valid JSON: %v", err)
	}
}

func TestReport_JSON_OmitsEmptyRemediation(t *testing.T) {
	r := &Report{
		Results: []CheckResult{
			{Name: "test", Status: "pass", Message: "ok"},
		},
	}

	out, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	// The "remediation" field should be omitted (omitempty tag).
	var raw map[string][]map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("JSON() output is not valid JSON: %v", err)
	}
	if _, exists := raw["results"][0]["remediation"]; exists {
		t.Error("JSON() should omit remediation when empty (omitempty)")
	}
}

func TestCheckResult_Fields(t *testing.T) {
	cr := CheckResult{
		Name:        "Test Check",
		Status:      "fail",
		Message:     "something is wrong",
		Remediation: "fix it",
	}

	if cr.Name != "Test Check" {
		t.Errorf("Name = %q, want %q", cr.Name, "Test Check")
	}
	if cr.Status != "fail" {
		t.Errorf("Status = %q, want %q", cr.Status, "fail")
	}
	if cr.Message != "something is wrong" {
		t.Errorf("Message = %q, want %q", cr.Message, "something is wrong")
	}
	if cr.Remediation != "fix it" {
		t.Errorf("Remediation = %q, want %q", cr.Remediation, "fix it")
	}

	// Verify JSON tags via marshal/unmarshal.
	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"name", "status", "message", "remediation"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("JSON output missing key %q", key)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"single line", "single line"},
		{"first\nsecond\nthird", "first"},
		{"", ""},
		{"trailing\n", "trailing"},
	}

	for _, tt := range tests {
		got := firstLine(tt.input)
		if got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
