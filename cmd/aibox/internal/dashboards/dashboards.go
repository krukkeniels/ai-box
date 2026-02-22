// Package dashboards generates Grafana dashboard definitions and alert rules
// for AI-Box operational monitoring.
//
// Three dashboards are provided:
//   - Platform Operations: fleet health, startup times, image versions
//   - Security Posture: blocked connections, policy violations, Falco alerts
//   - Executive Summary: adoption rate, incident count, cost per developer
//
// Dashboards are provisioned as code using Grafana's provisioning format.
// See SPEC-FINAL.md Sections 19.1 and 22.1.
package dashboards

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds dashboard provisioning configuration.
type Config struct {
	ProvisioningDir string // directory for Grafana provisioning files (default "/etc/grafana/provisioning/dashboards")
	AlertRulesDir   string // directory for alert rule provisioning (default "/etc/grafana/provisioning/alerting")
	DataSourceName  string // Grafana data source name (default "AI-Box Logs")
	OrgID           int    // Grafana organization ID (default 1)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ProvisioningDir: "/etc/grafana/provisioning/dashboards",
		AlertRulesDir:   "/etc/grafana/provisioning/alerting",
		DataSourceName:  "AI-Box Logs",
		OrgID:           1,
	}
}

// Manager manages dashboard provisioning.
type Manager struct {
	cfg Config
}

// NewManager creates a Manager, applying defaults for any zero-value fields.
func NewManager(cfg Config) *Manager {
	if cfg.ProvisioningDir == "" {
		cfg.ProvisioningDir = "/etc/grafana/provisioning/dashboards"
	}
	if cfg.AlertRulesDir == "" {
		cfg.AlertRulesDir = "/etc/grafana/provisioning/alerting"
	}
	if cfg.DataSourceName == "" {
		cfg.DataSourceName = "AI-Box Logs"
	}
	if cfg.OrgID == 0 {
		cfg.OrgID = 1
	}
	return &Manager{cfg: cfg}
}

// Config returns the current configuration (read-only copy).
func (m *Manager) Config() Config {
	return m.cfg
}

// Dashboard represents a Grafana dashboard definition.
type Dashboard struct {
	UID         string  `json:"uid"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Tags        []string `json:"tags"`
	Panels      []Panel `json:"panels"`
	Refresh     string  `json:"refresh,omitempty"`
	SchemaVer   int     `json:"schemaVersion"`
}

// Panel represents a single dashboard panel.
type Panel struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`       // stat, timeseries, table, gauge, bargauge, text
	DataSource  string         `json:"datasource"` // data source name
	GridPos     GridPos        `json:"gridPos"`
	Description string         `json:"description,omitempty"`
	Targets     []Target       `json:"targets,omitempty"`
	FieldConfig *FieldConfig   `json:"fieldConfig,omitempty"`
}

// GridPos defines panel position and size in the dashboard grid.
type GridPos struct {
	H int `json:"h"` // height
	W int `json:"w"` // width (max 24)
	X int `json:"x"` // x position
	Y int `json:"y"` // y position
}

// Target represents a query target for a panel.
type Target struct {
	Expr       string `json:"expr,omitempty"`       // PromQL/LogQL expression
	LegendFmt string `json:"legendFormat,omitempty"`
	RefID      string `json:"refId"`
}

// FieldConfig holds panel field configuration.
type FieldConfig struct {
	Defaults FieldDefaults `json:"defaults"`
}

// FieldDefaults holds default field settings.
type FieldDefaults struct {
	Unit       string      `json:"unit,omitempty"`
	Thresholds *Thresholds `json:"thresholds,omitempty"`
}

// Thresholds defines threshold steps for panel coloring.
type Thresholds struct {
	Mode  string          `json:"mode"` // "absolute" or "percentage"
	Steps []ThresholdStep `json:"steps"`
}

// ThresholdStep defines a single threshold step.
type ThresholdStep struct {
	Color string   `json:"color"`
	Value *float64 `json:"value"` // nil for base step
}

// AlertRule represents a Grafana alerting rule.
type AlertRule struct {
	Name        string `json:"name"`
	Condition   string `json:"condition"`
	Severity    string `json:"severity"`    // critical, high, medium, info
	Channel     string `json:"channel"`     // pagerduty, slack, email, dashboard
	SLA         string `json:"sla"`         // response time requirement
	Description string `json:"description"`
	Expr        string `json:"expr"`        // alert expression
}

// ProvisioningConfig is the Grafana dashboard provisioning YAML structure.
type ProvisioningConfig struct {
	APIVersion  int                  `json:"apiVersion" yaml:"apiVersion"`
	Providers   []ProvisioningEntry  `json:"providers" yaml:"providers"`
}

// ProvisioningEntry is a single provisioning provider entry.
type ProvisioningEntry struct {
	Name      string                 `json:"name" yaml:"name"`
	OrgID     int                    `json:"orgId" yaml:"orgId"`
	Folder    string                 `json:"folder" yaml:"folder"`
	Type      string                 `json:"type" yaml:"type"`
	Options   map[string]interface{} `json:"options" yaml:"options"`
}

// PlatformOpsDashboard returns the Platform Operations dashboard definition.
func (m *Manager) PlatformOpsDashboard() Dashboard {
	ds := m.cfg.DataSourceName
	return Dashboard{
		UID:         "aibox-platform-ops",
		Title:       "AI-Box Platform Operations",
		Description: "Fleet health, startup performance, and operational metrics for AI-Box sandboxes",
		Tags:        []string{"aibox", "operations", "platform"},
		Refresh:     "30s",
		SchemaVer:   39,
		Panels: []Panel{
			{
				ID: 1, Title: "Active Sandboxes", Type: "stat",
				DataSource: ds,
				GridPos:    GridPos{H: 4, W: 6, X: 0, Y: 0},
				Description: "Total number of currently running AI-Box sandboxes",
				Targets: []Target{
					{Expr: `count(aibox_sandbox_status{status="running"})`, RefID: "A"},
				},
			},
			{
				ID: 2, Title: "Active Sandboxes by Team", Type: "bargauge",
				DataSource: ds,
				GridPos:    GridPos{H: 4, W: 6, X: 6, Y: 0},
				Description: "Running sandboxes grouped by team label",
				Targets: []Target{
					{Expr: `count by (team) (aibox_sandbox_status{status="running"})`, RefID: "A"},
				},
			},
			{
				ID: 3, Title: "Startup Time Distribution", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 12, Y: 0},
				Description: "Sandbox startup time percentiles (SLA: cold <90s, warm <15s)",
				Targets: []Target{
					{Expr: `histogram_quantile(0.50, rate(aibox_sandbox_startup_seconds_bucket[5m]))`, LegendFmt: "p50", RefID: "A"},
					{Expr: `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket[5m]))`, LegendFmt: "p95", RefID: "B"},
					{Expr: `histogram_quantile(0.99, rate(aibox_sandbox_startup_seconds_bucket[5m]))`, LegendFmt: "p99", RefID: "C"},
				},
				FieldConfig: &FieldConfig{Defaults: FieldDefaults{Unit: "s"}},
			},
			{
				ID: 4, Title: "Image Versions in Use", Type: "table",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 12, X: 0, Y: 4},
				Description: "Container image versions currently deployed across the fleet",
				Targets: []Target{
					{Expr: `count by (image, version) (aibox_sandbox_status{status="running"})`, RefID: "A"},
				},
			},
			{
				ID: 5, Title: "Doctor Check Failure Rate", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 12, X: 12, Y: 8},
				Description: "Rate of aibox doctor check failures by check name",
				Targets: []Target{
					{Expr: `rate(aibox_doctor_check_failures_total[1h])`, LegendFmt: "{{check}}", RefID: "A"},
				},
			},
			{
				ID: 6, Title: "Tool Pack Installations", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 12, X: 0, Y: 10},
				Description: "Tool pack installation frequency (demand signals for pre-built images)",
				Targets: []Target{
					{Expr: `increase(aibox_toolpack_installs_total[24h])`, LegendFmt: "{{pack}}", RefID: "A"},
				},
			},
		},
	}
}

// SecurityPostureDashboard returns the Security Posture dashboard definition.
func (m *Manager) SecurityPostureDashboard() Dashboard {
	ds := m.cfg.DataSourceName
	return Dashboard{
		UID:         "aibox-security-posture",
		Title:       "AI-Box Security Posture",
		Description: "Security monitoring: blocked connections, policy violations, Falco alerts, and threat indicators",
		Tags:        []string{"aibox", "security", "monitoring"},
		Refresh:     "30s",
		SchemaVer:   39,
		Panels: []Panel{
			{
				ID: 1, Title: "Blocked Network Attempts", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 0, Y: 0},
				Description: "Rate of blocked outbound network attempts by sandbox (exfiltration detection)",
				Targets: []Target{
					{Expr: `rate(aibox_network_blocked_total[5m])`, LegendFmt: "{{sandbox_id}}", RefID: "A"},
				},
			},
			{
				ID: 2, Title: "DNS Query Patterns", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 12, Y: 0},
				Description: "DNS query rate and entropy score per sandbox (tunneling detection)",
				Targets: []Target{
					{Expr: `rate(aibox_dns_queries_total[5m])`, LegendFmt: "queries {{sandbox_id}}", RefID: "A"},
					{Expr: `aibox_dns_entropy_score`, LegendFmt: "entropy {{sandbox_id}}", RefID: "B"},
				},
			},
			{
				ID: 3, Title: "Policy Violations", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 0, Y: 8},
				Description: "OPA policy violation rate by rule and team",
				Targets: []Target{
					{Expr: `rate(aibox_policy_violations_total[5m])`, LegendFmt: "{{rule}} ({{team}})", RefID: "A"},
				},
			},
			{
				ID: 4, Title: "Falco Alerts", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 12, Y: 8},
				Description: "Falco runtime security alerts by severity and rule",
				Targets: []Target{
					{Expr: `rate(aibox_falco_alerts_total[5m])`, LegendFmt: "{{rule}} [{{severity}}]", RefID: "A"},
				},
			},
			{
				ID: 5, Title: "LLM API Usage", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 0, Y: 16},
				Description: "LLM API request rate, token counts, and payload sizes",
				Targets: []Target{
					{Expr: `rate(aibox_llm_requests_total[5m])`, LegendFmt: "requests", RefID: "A"},
					{Expr: `rate(aibox_llm_payload_bytes_sum[5m])`, LegendFmt: "bytes/s", RefID: "B"},
				},
			},
			{
				ID: 6, Title: "Credential Lifecycle", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 8, W: 12, X: 12, Y: 16},
				Description: "Credential issuance, revocation, and TTL adherence",
				Targets: []Target{
					{Expr: `rate(aibox_credential_issued_total[1h])`, LegendFmt: "issued", RefID: "A"},
					{Expr: `rate(aibox_credential_revoked_total[1h])`, LegendFmt: "revoked", RefID: "B"},
				},
			},
			{
				ID: 7, Title: "Session Recording Coverage", Type: "gauge",
				DataSource: ds,
				GridPos:    GridPos{H: 4, W: 6, X: 0, Y: 24},
				Description: "Percentage of sandboxes with active session recording",
				Targets: []Target{
					{Expr: `aibox_recording_active / aibox_sandbox_total * 100`, RefID: "A"},
				},
				FieldConfig: &FieldConfig{
					Defaults: FieldDefaults{
						Unit: "percent",
						Thresholds: &Thresholds{
							Mode: "absolute",
							Steps: []ThresholdStep{
								{Color: "red", Value: nil},
								{Color: "yellow", Value: float64Ptr(50)},
								{Color: "green", Value: float64Ptr(90)},
							},
						},
					},
				},
			},
		},
	}
}

// ExecutiveDashboard returns the Executive Summary dashboard definition.
func (m *Manager) ExecutiveDashboard() Dashboard {
	ds := m.cfg.DataSourceName
	return Dashboard{
		UID:         "aibox-executive",
		Title:       "AI-Box Executive Summary",
		Description: "High-level adoption, security, and cost metrics for engineering leadership",
		Tags:        []string{"aibox", "executive", "summary"},
		Refresh:     "1h",
		SchemaVer:   39,
		Panels: []Panel{
			{
				ID: 1, Title: "Adoption Rate", Type: "gauge",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 6, X: 0, Y: 0},
				Description: "Percentage of developers actively using AI-Box",
				Targets: []Target{
					{Expr: `aibox_active_users / aibox_total_developers * 100`, RefID: "A"},
				},
				FieldConfig: &FieldConfig{
					Defaults: FieldDefaults{
						Unit: "percent",
						Thresholds: &Thresholds{
							Mode: "absolute",
							Steps: []ThresholdStep{
								{Color: "red", Value: nil},
								{Color: "yellow", Value: float64Ptr(30)},
								{Color: "green", Value: float64Ptr(70)},
							},
						},
					},
				},
			},
			{
				ID: 2, Title: "Security Incidents (30d)", Type: "stat",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 6, X: 6, Y: 0},
				Description: "Total security incidents in the last 30 days by severity",
				Targets: []Target{
					{Expr: `sum(increase(aibox_security_incidents_total[30d]))`, RefID: "A"},
				},
				FieldConfig: &FieldConfig{
					Defaults: FieldDefaults{
						Thresholds: &Thresholds{
							Mode: "absolute",
							Steps: []ThresholdStep{
								{Color: "green", Value: nil},
								{Color: "yellow", Value: float64Ptr(5)},
								{Color: "red", Value: float64Ptr(10)},
							},
						},
					},
				},
			},
			{
				ID: 3, Title: "Security Incident Trend", Type: "timeseries",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 12, X: 12, Y: 0},
				Description: "Security incident trend over time by severity",
				Targets: []Target{
					{Expr: `sum by (severity) (increase(aibox_security_incidents_total[7d]))`, LegendFmt: "{{severity}}", RefID: "A"},
				},
			},
			{
				ID: 4, Title: "Cost per Developer", Type: "stat",
				DataSource: ds,
				GridPos:    GridPos{H: 6, W: 6, X: 0, Y: 6},
				Description: "Estimated monthly cost per developer (compute, storage, ops)",
				Targets: []Target{
					{Expr: `aibox_monthly_cost_total / aibox_active_users`, RefID: "A"},
				},
				FieldConfig: &FieldConfig{Defaults: FieldDefaults{Unit: "currencyUSD"}},
			},
		},
	}
}

// AlertRules returns the configured alert rules with routing information.
func (m *Manager) AlertRules() []AlertRule {
	return []AlertRule{
		{
			Name:        "Container Escape Indicator",
			Condition:   "Falco fires critical-severity rule",
			Severity:    "critical",
			Channel:     "pagerduty",
			SLA:         "15 minutes",
			Description: "A Falco critical-severity rule has fired, indicating a potential container escape attempt",
			Expr:        `sum(rate(aibox_falco_alerts_total{severity="critical"}[5m])) > 0`,
		},
		{
			Name:        "Anomalous Outbound Data Volume",
			Condition:   "Proxy logs show >100MB transferred in 10 minutes from a single sandbox",
			Severity:    "high",
			Channel:     "slack",
			SLA:         "4 hours",
			Description: "A sandbox is transferring an unusually large volume of data outbound",
			Expr:        `sum by (sandbox_id) (increase(aibox_proxy_bytes_total[10m])) > 104857600`,
		},
		{
			Name:        "Repeated Blocked Network Attempts",
			Condition:   "More than 50 blocked requests in 5 minutes from the same sandbox",
			Severity:    "high",
			Channel:     "slack",
			SLA:         "4 hours",
			Description: "A sandbox is repeatedly attempting blocked network connections",
			Expr:        `sum by (sandbox_id) (increase(aibox_network_blocked_total[5m])) > 50`,
		},
		{
			Name:        "Policy Violation Burst",
			Condition:   "More than 20 OPA denials in 5 minutes from the same sandbox",
			Severity:    "high",
			Channel:     "slack",
			SLA:         "4 hours",
			Description: "A sandbox is generating a burst of policy violations",
			Expr:        `sum by (sandbox_id) (increase(aibox_policy_violations_total[5m])) > 20`,
		},
		{
			Name:        "Credential Access After Stop",
			Condition:   "Vault token use detected after sandbox lifecycle shows stop event",
			Severity:    "critical",
			Channel:     "pagerduty",
			SLA:         "15 minutes",
			Description: "Credential use detected after the associated sandbox has been stopped",
			Expr:        `aibox_credential_use_after_stop > 0`,
		},
		{
			Name:        "DNS Query Spike",
			Condition:   "More than 100 DNS queries/minute from a sandbox",
			Severity:    "medium",
			Channel:     "email",
			SLA:         "next business day",
			Description: "A sandbox is generating an unusually high rate of DNS queries",
			Expr:        `sum by (sandbox_id) (rate(aibox_dns_queries_total[1m])) > 100`,
		},
		{
			Name:        "Off-Hours Credential Access",
			Condition:   "Vault token issuance outside 06:00-22:00 local time",
			Severity:    "medium",
			Channel:     "email",
			SLA:         "next business day",
			Description: "Credential issuance detected outside normal business hours",
			Expr:        `increase(aibox_credential_issued_offhours_total[1h]) > 0`,
		},
		{
			Name:        "LLM Payload Size Anomaly",
			Condition:   "LLM proxy request payload exceeds 95th percentile by 3x",
			Severity:    "medium",
			Channel:     "email",
			SLA:         "next business day",
			Description: "An LLM API request has an unusually large payload",
			Expr:        `aibox_llm_payload_bytes > 3 * aibox_llm_payload_p95`,
		},
		{
			Name:        "Git Push to Unexpected Remote",
			Condition:   "Git push operation to non-allowlisted remote detected",
			Severity:    "high",
			Channel:     "slack",
			SLA:         "4 hours",
			Description: "A git push to a non-allowlisted remote repository was attempted",
			Expr:        `increase(aibox_git_push_unexpected_total[5m]) > 0`,
		},
		{
			Name:        "Base64 Payload Anomaly",
			Condition:   "LLM proxy detects base64-encoded blocks in unusual request fields",
			Severity:    "medium",
			Channel:     "email",
			SLA:         "next business day",
			Description: "Base64-encoded content detected in unexpected LLM API request fields",
			Expr:        `increase(aibox_llm_base64_anomaly_total[1h]) > 0`,
		},
	}
}

// DashboardNames returns the names of all available dashboards.
func DashboardNames() []string {
	return []string{
		"aibox-platform-ops",
		"aibox-security-posture",
		"aibox-executive",
	}
}

// AlertSeverities returns valid alert severity levels and their routing channels.
func AlertSeverities() map[string]string {
	return map[string]string{
		"critical": "pagerduty",
		"high":     "slack",
		"medium":   "email",
		"info":     "dashboard",
	}
}

// AlertSLAs returns the SLA for each severity level.
func AlertSLAs() map[string]string {
	return map[string]string{
		"critical": "15 minutes",
		"high":     "4 hours",
		"medium":   "next business day",
		"info":     "weekly review",
	}
}

// GenerateJSON returns the dashboard as a formatted JSON string suitable for
// Grafana provisioning.
func GenerateJSON(d Dashboard) (string, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling dashboard %s: %w", d.UID, err)
	}
	return string(data), nil
}

// WriteProvisioningFiles writes all dashboard JSON files and the provisioning
// config to the configured directory.
func (m *Manager) WriteProvisioningFiles() error {
	if err := os.MkdirAll(m.cfg.ProvisioningDir, 0o755); err != nil {
		return fmt.Errorf("creating provisioning directory %s: %w", m.cfg.ProvisioningDir, err)
	}

	dashboards := []Dashboard{
		m.PlatformOpsDashboard(),
		m.SecurityPostureDashboard(),
		m.ExecutiveDashboard(),
	}

	for _, d := range dashboards {
		jsonData, err := GenerateJSON(d)
		if err != nil {
			return err
		}
		path := filepath.Join(m.cfg.ProvisioningDir, d.UID+".json")
		if err := os.WriteFile(path, []byte(jsonData), 0o644); err != nil {
			return fmt.Errorf("writing dashboard %s to %s: %w", d.UID, path, err)
		}
		slog.Info("wrote dashboard", "uid", d.UID, "path", path)
	}

	return nil
}

// WriteAlertRules writes the alert rules configuration to the configured directory.
func (m *Manager) WriteAlertRules() error {
	if err := os.MkdirAll(m.cfg.AlertRulesDir, 0o755); err != nil {
		return fmt.Errorf("creating alert rules directory %s: %w", m.cfg.AlertRulesDir, err)
	}

	rules := m.AlertRules()
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling alert rules: %w", err)
	}

	path := filepath.Join(m.cfg.AlertRulesDir, "aibox-alerts.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing alert rules to %s: %w", path, err)
	}

	slog.Info("wrote alert rules", "path", path, "count", len(rules))
	return nil
}

func float64Ptr(v float64) *float64 {
	return &v
}
