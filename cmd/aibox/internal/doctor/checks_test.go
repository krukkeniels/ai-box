package doctor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/host"
	"github.com/aibox/aibox/internal/security"
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

func TestStatusConstants(t *testing.T) {
	if StatusPass != "pass" {
		t.Error("StatusPass should be 'pass'")
	}
	if StatusWarn != "warn" {
		t.Error("StatusWarn should be 'warn'")
	}
	if StatusFail != "fail" {
		t.Error("StatusFail should be 'fail'")
	}
	if StatusInfo != "info" {
		t.Error("StatusInfo should be 'info'")
	}
}

func TestReport_HasFailures_IgnoresInfo(t *testing.T) {
	report := &Report{
		Results: []CheckResult{
			{Status: StatusPass},
			{Status: StatusInfo},
		},
	}
	if report.HasFailures() {
		t.Error("info status should not count as failure")
	}
}

// TestRemediations_NoRepoRelativePaths verifies that doctor remediation messages
// do not reference repo-relative paths (which don't exist in binary installs)
// and instead direct users to "aibox setup --system".
func TestRemediations_NoRepoRelativePaths(t *testing.T) {
	// Paths that should never appear in remediation messages.
	forbiddenPaths := []string{
		"configs/seccomp.json",
		"configs/apparmor/",
		"aibox-policies/",
	}

	// CheckSeccomp: on a test machine without /etc/aibox/seccomp.json,
	// this will produce a fail result with remediation.
	seccompResult := CheckSeccomp()
	if seccompResult.Remediation != "" {
		for _, fp := range forbiddenPaths {
			if strings.Contains(seccompResult.Remediation, fp) {
				t.Errorf("CheckSeccomp remediation contains repo-relative path %q:\n  %s",
					fp, seccompResult.Remediation)
			}
		}
		if !strings.Contains(seccompResult.Remediation, "aibox setup --system") {
			t.Errorf("CheckSeccomp remediation should reference 'aibox setup --system', got:\n  %s",
				seccompResult.Remediation)
		}
	}

	// CheckAppArmor: exercise the check and verify remediation if present.
	apparmorResult := CheckAppArmor()
	if apparmorResult.Remediation != "" {
		for _, fp := range forbiddenPaths {
			if strings.Contains(apparmorResult.Remediation, fp) {
				t.Errorf("CheckAppArmor remediation contains repo-relative path %q:\n  %s",
					fp, apparmorResult.Remediation)
			}
		}
		// The "not loaded" remediation should use setup --system.
		// Other states (not available, error) have different messages.
		if strings.Contains(apparmorResult.Message, "not loaded") {
			if !strings.Contains(apparmorResult.Remediation, "aibox setup --system") {
				t.Errorf("CheckAppArmor 'not loaded' remediation should reference 'aibox setup --system', got:\n  %s",
					apparmorResult.Remediation)
			}
		}
	}

	// CheckPolicyFiles: use a config with a non-existent org baseline path
	// to trigger the missing-policy remediation.
	cfg := &config.Config{
		Policy: config.PolicyConfig{
			OrgBaselinePath: "/etc/aibox/org-policy.yaml",
		},
	}
	policyResult := CheckPolicyFiles(cfg)
	if policyResult.Remediation != "" {
		for _, fp := range forbiddenPaths {
			if strings.Contains(policyResult.Remediation, fp) {
				t.Errorf("CheckPolicyFiles remediation contains repo-relative path %q:\n  %s",
					fp, policyResult.Remediation)
			}
		}
		if !strings.Contains(policyResult.Remediation, "aibox setup --system") {
			t.Errorf("CheckPolicyFiles remediation should reference 'aibox setup --system', got:\n  %s",
				policyResult.Remediation)
		}
	}
}

// TestCheckSeccomp_RemediationContent verifies the exact remediation content
// when the seccomp profile is not found.
func TestCheckSeccomp_RemediationContent(t *testing.T) {
	result := CheckSeccomp()
	// If profile was found (CI or dev machine with it installed), skip.
	if result.Status == "pass" {
		t.Skip("seccomp profile found on this system; cannot test missing-profile remediation")
	}

	if !strings.Contains(result.Remediation, "bundled in the aibox binary") {
		t.Errorf("seccomp remediation should mention embedded asset, got:\n  %s", result.Remediation)
	}
	if !strings.Contains(result.Remediation, "aibox setup --system") {
		t.Errorf("seccomp remediation should reference 'aibox setup --system', got:\n  %s", result.Remediation)
	}
}

// TestCheckPolicyFiles_RemediationIncludesConfigPath verifies that the policy
// remediation references the configured org baseline path.
func TestCheckPolicyFiles_RemediationIncludesConfigPath(t *testing.T) {
	customPath := "/opt/custom/org-policy.yaml"
	cfg := &config.Config{
		Policy: config.PolicyConfig{
			OrgBaselinePath: customPath,
		},
	}
	result := CheckPolicyFiles(cfg)
	if result.Status == "pass" {
		t.Skip("policy file found; cannot test missing-policy remediation")
	}

	if !strings.Contains(result.Remediation, customPath) {
		t.Errorf("policy remediation should include configured path %q, got:\n  %s",
			customPath, result.Remediation)
	}
}

func TestCheckAppArmor_WSL_IsInfo(t *testing.T) {
	wslHost := host.HostInfo{OS: "linux", IsWSL2: true}
	result := CheckAppArmor(wslHost)
	// On this test machine AppArmor may or may not be available.
	// If unavailable on WSL, should be info.
	if !security.IsAppArmorAvailable() {
		if result.Status != StatusInfo {
			t.Errorf("AppArmor unavailable on WSL should be info, got %q", result.Status)
		}
		if !strings.Contains(result.Message, "expected on WSL2") {
			t.Error("message should mention WSL2")
		}
	}
}

func TestCheckAppArmor_NonWSL_IsWarn(t *testing.T) {
	nativeHost := host.HostInfo{OS: "linux", IsWSL2: false}
	result := CheckAppArmor(nativeHost)
	if !security.IsAppArmorAvailable() {
		if result.Status != StatusWarn {
			t.Errorf("AppArmor unavailable on native Linux should be warn, got %q", result.Status)
		}
	}
}

func TestCheckPolicyFiles_MinimalConfig_IsInfo(t *testing.T) {
	cfg := &config.Config{
		Policy: config.PolicyConfig{
			OrgBaselinePath: "/etc/aibox/org-policy.yaml",
		},
		Network: config.NetworkConfig{Enabled: false},
	}
	result := CheckPolicyFiles(cfg)
	// For minimal config with default policy path and network disabled,
	// missing policy should be info not warn.
	if result.Status == StatusWarn {
		t.Error("missing policy in minimal mode should not be warn")
	}
}

func TestRunAllWithOptions_Strict(t *testing.T) {
	cfg := &config.Config{}
	report := RunAllWithOptions(cfg, RunOptions{Strict: true})
	for _, r := range report.Results {
		if r.Status == StatusInfo {
			t.Errorf("strict mode should not have info-level results, found: %s", r.Name)
		}
	}
}
