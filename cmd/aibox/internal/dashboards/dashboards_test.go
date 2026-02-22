package dashboards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ProvisioningDir != "/etc/grafana/provisioning/dashboards" {
		t.Errorf("ProvisioningDir = %q, want /etc/grafana/provisioning/dashboards", cfg.ProvisioningDir)
	}
	if cfg.AlertRulesDir != "/etc/grafana/provisioning/alerting" {
		t.Errorf("AlertRulesDir = %q, want /etc/grafana/provisioning/alerting", cfg.AlertRulesDir)
	}
	if cfg.DataSourceName != "AI-Box Logs" {
		t.Errorf("DataSourceName = %q, want AI-Box Logs", cfg.DataSourceName)
	}
	if cfg.OrgID != 1 {
		t.Errorf("OrgID = %d, want 1", cfg.OrgID)
	}
}

func TestNewManager_DefaultsFilled(t *testing.T) {
	mgr := NewManager(Config{})

	cfg := mgr.Config()
	if cfg.ProvisioningDir == "" {
		t.Error("ProvisioningDir should have default")
	}
	if cfg.AlertRulesDir == "" {
		t.Error("AlertRulesDir should have default")
	}
	if cfg.DataSourceName == "" {
		t.Error("DataSourceName should have default")
	}
	if cfg.OrgID == 0 {
		t.Error("OrgID should have default")
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	mgr := NewManager(Config{
		ProvisioningDir: "/custom/provisioning",
		AlertRulesDir:   "/custom/alerting",
		DataSourceName:  "Custom DS",
		OrgID:           42,
	})

	cfg := mgr.Config()
	if cfg.ProvisioningDir != "/custom/provisioning" {
		t.Errorf("ProvisioningDir = %q, want /custom/provisioning", cfg.ProvisioningDir)
	}
	if cfg.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", cfg.OrgID)
	}
}

func TestDashboardNames_HasThree(t *testing.T) {
	names := DashboardNames()
	if len(names) != 3 {
		t.Errorf("expected 3 dashboard names, got %d", len(names))
	}
}

func TestDashboardNames_AllPrefixed(t *testing.T) {
	for _, name := range DashboardNames() {
		if !strings.HasPrefix(name, "aibox-") {
			t.Errorf("dashboard name %q should have aibox- prefix", name)
		}
	}
}

func TestPlatformOpsDashboard_Structure(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.PlatformOpsDashboard()

	if d.UID != "aibox-platform-ops" {
		t.Errorf("UID = %q, want aibox-platform-ops", d.UID)
	}
	if d.Title == "" {
		t.Error("Title should not be empty")
	}
	if len(d.Panels) < 5 {
		t.Errorf("expected at least 5 panels, got %d", len(d.Panels))
	}
	if d.Refresh != "30s" {
		t.Errorf("Refresh = %q, want 30s", d.Refresh)
	}
}

func TestPlatformOpsDashboard_RequiredPanels(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.PlatformOpsDashboard()

	requiredPanels := []string{
		"Active Sandboxes",
		"Startup Time",
		"Image Versions",
		"Doctor",
		"Tool Pack",
	}

	titles := make(map[string]bool)
	for _, p := range d.Panels {
		titles[p.Title] = true
	}

	for _, req := range requiredPanels {
		found := false
		for title := range titles {
			if strings.Contains(title, req) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing panel containing %q", req)
		}
	}
}

func TestPlatformOpsDashboard_PanelIDs(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.PlatformOpsDashboard()

	ids := make(map[int]bool)
	for _, p := range d.Panels {
		if ids[p.ID] {
			t.Errorf("duplicate panel ID: %d", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestSecurityPostureDashboard_Structure(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.SecurityPostureDashboard()

	if d.UID != "aibox-security-posture" {
		t.Errorf("UID = %q, want aibox-security-posture", d.UID)
	}
	if len(d.Panels) < 6 {
		t.Errorf("expected at least 6 panels, got %d", len(d.Panels))
	}
}

func TestSecurityPostureDashboard_RequiredPanels(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.SecurityPostureDashboard()

	requiredPanels := []string{
		"Blocked Network",
		"DNS",
		"Policy Violations",
		"Falco",
		"LLM",
		"Credential",
	}

	titles := make(map[string]bool)
	for _, p := range d.Panels {
		titles[p.Title] = true
	}

	for _, req := range requiredPanels {
		found := false
		for title := range titles {
			if strings.Contains(title, req) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing security panel containing %q", req)
		}
	}
}

func TestSecurityPostureDashboard_HasSessionRecording(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.SecurityPostureDashboard()

	found := false
	for _, p := range d.Panels {
		if strings.Contains(p.Title, "Recording") {
			found = true
			break
		}
	}
	if !found {
		t.Error("security dashboard should have Session Recording Coverage panel")
	}
}

func TestExecutiveDashboard_Structure(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.ExecutiveDashboard()

	if d.UID != "aibox-executive" {
		t.Errorf("UID = %q, want aibox-executive", d.UID)
	}
	if d.Refresh != "1h" {
		t.Errorf("Executive dashboard Refresh = %q, want 1h (not operational noise)", d.Refresh)
	}
	// Executive dashboard should be simple (4-5 panels).
	if len(d.Panels) < 3 || len(d.Panels) > 6 {
		t.Errorf("expected 3-6 panels for executive dashboard, got %d", len(d.Panels))
	}
}

func TestExecutiveDashboard_RequiredPanels(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	d := mgr.ExecutiveDashboard()

	requiredPanels := []string{
		"Adoption",
		"Incident",
		"Cost",
	}

	titles := make(map[string]bool)
	for _, p := range d.Panels {
		titles[p.Title] = true
	}

	for _, req := range requiredPanels {
		found := false
		for title := range titles {
			if strings.Contains(title, req) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing executive panel containing %q", req)
		}
	}
}

func TestAllDashboards_HaveAiboxTag(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	dashboards := []Dashboard{
		mgr.PlatformOpsDashboard(),
		mgr.SecurityPostureDashboard(),
		mgr.ExecutiveDashboard(),
	}

	for _, d := range dashboards {
		hasTag := false
		for _, tag := range d.Tags {
			if tag == "aibox" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("dashboard %q missing aibox tag", d.UID)
		}
	}
}

func TestAllDashboards_HaveUniqueUIDs(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	dashboards := []Dashboard{
		mgr.PlatformOpsDashboard(),
		mgr.SecurityPostureDashboard(),
		mgr.ExecutiveDashboard(),
	}

	uids := make(map[string]bool)
	for _, d := range dashboards {
		if uids[d.UID] {
			t.Errorf("duplicate dashboard UID: %s", d.UID)
		}
		uids[d.UID] = true
	}
}

func TestAllDashboards_PanelsHaveDataSource(t *testing.T) {
	mgr := NewManager(Config{DataSourceName: "Test DS"})
	dashboards := []Dashboard{
		mgr.PlatformOpsDashboard(),
		mgr.SecurityPostureDashboard(),
		mgr.ExecutiveDashboard(),
	}

	for _, d := range dashboards {
		for _, p := range d.Panels {
			if p.DataSource != "Test DS" {
				t.Errorf("panel %q in %q has DataSource = %q, want Test DS",
					p.Title, d.UID, p.DataSource)
			}
		}
	}
}

func TestAllDashboards_PanelsHaveTargets(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	dashboards := []Dashboard{
		mgr.PlatformOpsDashboard(),
		mgr.SecurityPostureDashboard(),
		mgr.ExecutiveDashboard(),
	}

	for _, d := range dashboards {
		for _, p := range d.Panels {
			if len(p.Targets) == 0 {
				t.Errorf("panel %q in %q has no targets", p.Title, d.UID)
			}
		}
	}
}

func TestAlertRules_Count(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	rules := mgr.AlertRules()

	if len(rules) < 8 {
		t.Errorf("expected at least 8 alert rules, got %d", len(rules))
	}
}

func TestAlertRules_AllHaveRequiredFields(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	rules := mgr.AlertRules()

	for _, r := range rules {
		if r.Name == "" {
			t.Error("alert rule missing Name")
		}
		if r.Severity == "" {
			t.Errorf("alert rule %q missing Severity", r.Name)
		}
		if r.Channel == "" {
			t.Errorf("alert rule %q missing Channel", r.Name)
		}
		if r.SLA == "" {
			t.Errorf("alert rule %q missing SLA", r.Name)
		}
		if r.Expr == "" {
			t.Errorf("alert rule %q missing Expr", r.Name)
		}
	}
}

func TestAlertRules_SeverityChannelMapping(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	rules := mgr.AlertRules()
	expectedChannels := AlertSeverities()

	for _, r := range rules {
		expected, ok := expectedChannels[r.Severity]
		if !ok {
			t.Errorf("alert rule %q has unknown severity %q", r.Name, r.Severity)
			continue
		}
		if r.Channel != expected {
			t.Errorf("alert rule %q: severity %q should route to %q, got %q",
				r.Name, r.Severity, expected, r.Channel)
		}
	}
}

func TestAlertRules_SLAMapping(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	rules := mgr.AlertRules()
	expectedSLAs := AlertSLAs()

	for _, r := range rules {
		expected, ok := expectedSLAs[r.Severity]
		if !ok {
			continue // already checked in severity test
		}
		if r.SLA != expected {
			t.Errorf("alert rule %q: severity %q should have SLA %q, got %q",
				r.Name, r.Severity, expected, r.SLA)
		}
	}
}

func TestAlertRules_HasCriticalRules(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	rules := mgr.AlertRules()

	criticalCount := 0
	for _, r := range rules {
		if r.Severity == "critical" {
			criticalCount++
		}
	}
	if criticalCount < 2 {
		t.Errorf("expected at least 2 critical rules, got %d", criticalCount)
	}
}

func TestAlertSeverities_AllLevels(t *testing.T) {
	sev := AlertSeverities()

	expectedLevels := []string{"critical", "high", "medium", "info"}
	for _, level := range expectedLevels {
		if _, ok := sev[level]; !ok {
			t.Errorf("missing severity level %q", level)
		}
	}
}

func TestAlertSLAs_AllLevels(t *testing.T) {
	slas := AlertSLAs()

	expectedLevels := []string{"critical", "high", "medium", "info"}
	for _, level := range expectedLevels {
		if _, ok := slas[level]; !ok {
			t.Errorf("missing SLA for severity level %q", level)
		}
	}
}

func TestGenerateJSON_ValidJSON(t *testing.T) {
	mgr := NewManager(DefaultConfig())
	dashboards := []Dashboard{
		mgr.PlatformOpsDashboard(),
		mgr.SecurityPostureDashboard(),
		mgr.ExecutiveDashboard(),
	}

	for _, d := range dashboards {
		jsonStr, err := GenerateJSON(d)
		if err != nil {
			t.Errorf("GenerateJSON(%q) failed: %v", d.UID, err)
			continue
		}
		if jsonStr == "" {
			t.Errorf("GenerateJSON(%q) returned empty string", d.UID)
			continue
		}
		// Verify it's valid JSON.
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			t.Errorf("GenerateJSON(%q) produced invalid JSON: %v", d.UID, err)
		}
	}
}

func TestWriteProvisioningFiles_CreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		ProvisioningDir: filepath.Join(tmpDir, "dashboards"),
	})

	if err := mgr.WriteProvisioningFiles(); err != nil {
		t.Fatalf("WriteProvisioningFiles failed: %v", err)
	}

	for _, uid := range DashboardNames() {
		path := filepath.Join(tmpDir, "dashboards", uid+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("dashboard file not created: %s: %v", path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("dashboard file is empty: %s", path)
		}
		// Verify it's valid JSON.
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Errorf("dashboard file %s contains invalid JSON: %v", path, err)
		}
	}
}

func TestWriteAlertRules_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		AlertRulesDir: filepath.Join(tmpDir, "alerting"),
	})

	if err := mgr.WriteAlertRules(); err != nil {
		t.Fatalf("WriteAlertRules failed: %v", err)
	}

	path := filepath.Join(tmpDir, "alerting", "aibox-alerts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("alert rules file not created: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("alert rules file is empty")
	}

	// Verify it's valid JSON array.
	var rules []AlertRule
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatalf("alert rules file contains invalid JSON: %v", err)
	}
	if len(rules) < 8 {
		t.Errorf("expected at least 8 alert rules in file, got %d", len(rules))
	}
}
