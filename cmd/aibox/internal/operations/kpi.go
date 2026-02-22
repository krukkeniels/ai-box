package operations

import (
	"fmt"
	"time"

	"github.com/aibox/aibox/internal/dashboards"
)

// KPI represents a key performance indicator with its current value and target.
type KPI struct {
	Name   string  // human-readable name
	Value  float64 // current value
	Target float64 // target value
	Unit   string  // unit of measurement (percent, seconds, count)
	Met    bool    // whether the target is met
}

// KPISnapshot holds a point-in-time collection of all tracked KPIs.
type KPISnapshot struct {
	Timestamp        time.Time
	AdoptionRate     KPI
	ColdStartP95     KPI
	WarmStartP95     KPI
	SupportTickets   KPI
	FallbackRate     KPI
	SecurityEvents   KPI
	CutoverReady     bool   // true if all mandatory cutover conditions are met
	CutoverReadyText string // human-readable status
}

// KPIConfig holds thresholds and targets for KPI evaluation.
type KPIConfig struct {
	TotalDevelopers       int     // total registered developers
	AdoptionTarget        float64 // target adoption percentage (default 90)
	ColdStartTarget       float64 // target cold start p95 in seconds (default 90)
	WarmStartTarget       float64 // target warm start p95 in seconds (default 15)
	TicketTarget          float64 // target weekly ticket count (default 3)
	FallbackTarget        float64 // target fallback percentage (default 5)
	CutoverAdoptionWeeks  int     // weeks adoption must sustain (default 2)
	CutoverTicketWeeks    int     // weeks tickets must sustain (default 4)
}

// DefaultKPIConfig returns configuration with targets from SPEC-FINAL.md.
func DefaultKPIConfig() KPIConfig {
	return KPIConfig{
		TotalDevelopers:      200,
		AdoptionTarget:       90.0,
		ColdStartTarget:      90.0,
		WarmStartTarget:      15.0,
		TicketTarget:         3.0,
		FallbackTarget:       5.0,
		CutoverAdoptionWeeks: 2,
		CutoverTicketWeeks:   4,
	}
}

// NewAdoptionKPI creates an adoption rate KPI from raw values.
func NewAdoptionKPI(activeUsers, totalDevelopers int) KPI {
	if totalDevelopers == 0 {
		return KPI{Name: "Adoption Rate", Unit: "percent", Target: 90.0}
	}
	rate := float64(activeUsers) / float64(totalDevelopers) * 100.0
	return KPI{
		Name:   "Adoption Rate",
		Value:  rate,
		Target: 90.0,
		Unit:   "percent",
		Met:    rate >= 90.0,
	}
}

// NewStartupKPI creates a startup time KPI.
func NewStartupKPI(name string, p95Seconds, targetSeconds float64) KPI {
	return KPI{
		Name:   name,
		Value:  p95Seconds,
		Target: targetSeconds,
		Unit:   "seconds",
		Met:    p95Seconds <= targetSeconds,
	}
}

// NewTicketKPI creates a support ticket volume KPI.
func NewTicketKPI(weeklyCount int) KPI {
	return KPI{
		Name:   "Support Tickets (7d)",
		Value:  float64(weeklyCount),
		Target: 3.0,
		Unit:   "count",
		Met:    weeklyCount <= 3,
	}
}

// NewFallbackKPI creates a fallback frequency KPI.
func NewFallbackKPI(fallbacks, totalStarts int) KPI {
	if totalStarts == 0 {
		return KPI{Name: "Fallback Rate", Unit: "percent", Target: 5.0, Met: true}
	}
	rate := float64(fallbacks) / float64(totalStarts) * 100.0
	return KPI{
		Name:   "Fallback Rate",
		Value:  rate,
		Target: 5.0,
		Unit:   "percent",
		Met:    rate <= 5.0,
	}
}

// NewSecurityEventsKPI creates a security events KPI.
func NewSecurityEventsKPI(count int) KPI {
	return KPI{
		Name:  "Security Events (7d)",
		Value: float64(count),
		Unit:  "count",
		// No fixed target; trend should be downward.
	}
}

// CheckCutoverReadiness evaluates whether the mandatory cutover conditions
// are met per PO decision: adoption >90% for 2 weeks, tickets <3/week for
// 4 weeks, all teams have validated tool pack config.
func CheckCutoverReadiness(
	adoptionWeeks []float64, // adoption rate for each of the last N weeks
	ticketWeeks []int, // ticket count for each of the last N weeks
	allTeamsValidated bool,
	cfg KPIConfig,
) (ready bool, status string) {
	adoptionOK := false
	if len(adoptionWeeks) >= cfg.CutoverAdoptionWeeks {
		adoptionOK = true
		for i := len(adoptionWeeks) - cfg.CutoverAdoptionWeeks; i < len(adoptionWeeks); i++ {
			if adoptionWeeks[i] < cfg.AdoptionTarget {
				adoptionOK = false
				break
			}
		}
	}

	ticketsOK := false
	if len(ticketWeeks) >= cfg.CutoverTicketWeeks {
		ticketsOK = true
		for i := len(ticketWeeks) - cfg.CutoverTicketWeeks; i < len(ticketWeeks); i++ {
			if ticketWeeks[i] > int(cfg.TicketTarget) {
				ticketsOK = false
				break
			}
		}
	}

	conditions := 0
	parts := []string{}
	if adoptionOK {
		conditions++
		parts = append(parts, "adoption OK")
	} else {
		parts = append(parts, "adoption NOT MET")
	}
	if ticketsOK {
		conditions++
		parts = append(parts, "tickets OK")
	} else {
		parts = append(parts, "tickets NOT MET")
	}
	if allTeamsValidated {
		conditions++
		parts = append(parts, "teams OK")
	} else {
		parts = append(parts, "teams NOT MET")
	}

	ready = conditions == 3
	status = fmt.Sprintf("%d/3 conditions met (%s)", conditions, joinStrings(parts))
	return ready, status
}

func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for i := 1; i < len(ss); i++ {
		result += ", " + ss[i]
	}
	return result
}

// WeeklyReport generates a formatted weekly KPI report string suitable for
// posting to Slack.
func WeeklyReport(snap KPISnapshot) string {
	cutoverStatus := "NOT READY"
	if snap.CutoverReady {
		cutoverStatus = "READY"
	}

	return fmt.Sprintf(
		"AI-Box Weekly KPIs (Week of %s)\n"+
			"- Adoption: %.1f%% (target: >%.0f%%)\n"+
			"- Startup p95: %.1fs cold / %.1fs warm (target: <%.0fs / <%.0fs)\n"+
			"- Support threads: %.0f (target: <%.0f)\n"+
			"- Fallback rate: %.1f%% (target: <%.0f%%)\n"+
			"- Security events: %.0f\n"+
			"- Mandatory cutover: %s (%s)",
		snap.Timestamp.Format("2006-01-02"),
		snap.AdoptionRate.Value, snap.AdoptionRate.Target,
		snap.ColdStartP95.Value, snap.WarmStartP95.Value,
		snap.ColdStartP95.Target, snap.WarmStartP95.Target,
		snap.SupportTickets.Value, snap.SupportTickets.Target,
		snap.FallbackRate.Value, snap.FallbackRate.Target,
		snap.SecurityEvents.Value,
		cutoverStatus, snap.CutoverReadyText,
	)
}

// KPIDashboard returns a Grafana dashboard definition for KPI tracking.
// This integrates with the Phase 5 dashboard framework in internal/dashboards.
func KPIDashboard(dataSource string) dashboards.Dashboard {
	if dataSource == "" {
		dataSource = "AI-Box Logs"
	}
	return dashboards.Dashboard{
		UID:         "aibox-kpi-tracking",
		Title:       "AI-Box KPI Tracking",
		Description: "Rollout KPIs: adoption, startup performance, support volume, fallback rate, security events",
		Tags:        []string{"aibox", "kpi", "rollout"},
		Refresh:     "15m",
		SchemaVer:   39,
		Panels: []dashboards.Panel{
			{
				ID: 1, Title: "Adoption Rate (7d)", Type: "gauge",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 6, W: 6, X: 0, Y: 0},
				Description: "Percentage of developers active in the last 7 days",
				Targets: []dashboards.Target{
					{Expr: `aibox_active_users_7d / aibox_total_developers * 100`, RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{
					Defaults: dashboards.FieldDefaults{
						Unit: "percent",
						Thresholds: &dashboards.Thresholds{
							Mode: "absolute",
							Steps: []dashboards.ThresholdStep{
								{Color: "red", Value: nil},
								{Color: "yellow", Value: float64Ptr(50)},
								{Color: "green", Value: float64Ptr(90)},
							},
						},
					},
				},
			},
			{
				ID: 2, Title: "Cold Start p95", Type: "stat",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 6, W: 6, X: 6, Y: 0},
				Description: "95th percentile cold start time (SLA: <90s)",
				Targets: []dashboards.Target{
					{Expr: `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="cold"}[24h]))`, RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{
					Defaults: dashboards.FieldDefaults{
						Unit: "s",
						Thresholds: &dashboards.Thresholds{
							Mode: "absolute",
							Steps: []dashboards.ThresholdStep{
								{Color: "green", Value: nil},
								{Color: "yellow", Value: float64Ptr(60)},
								{Color: "red", Value: float64Ptr(90)},
							},
						},
					},
				},
			},
			{
				ID: 3, Title: "Support Threads (7d)", Type: "stat",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 6, W: 6, X: 12, Y: 0},
				Description: "Support thread count in the last 7 days",
				Targets: []dashboards.Target{
					{Expr: `increase(aibox_support_threads_total[7d])`, RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{
					Defaults: dashboards.FieldDefaults{
						Thresholds: &dashboards.Thresholds{
							Mode: "absolute",
							Steps: []dashboards.ThresholdStep{
								{Color: "green", Value: nil},
								{Color: "yellow", Value: float64Ptr(3)},
								{Color: "red", Value: float64Ptr(10)},
							},
						},
					},
				},
			},
			{
				ID: 4, Title: "Fallback Rate (7d)", Type: "gauge",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 6, W: 6, X: 18, Y: 0},
				Description: "Percentage of sessions falling back to local dev",
				Targets: []dashboards.Target{
					{Expr: `increase(aibox_fallback_events_total[7d]) / increase(aibox_sandbox_starts_total[7d]) * 100`, RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{
					Defaults: dashboards.FieldDefaults{
						Unit: "percent",
						Thresholds: &dashboards.Thresholds{
							Mode: "absolute",
							Steps: []dashboards.ThresholdStep{
								{Color: "green", Value: nil},
								{Color: "yellow", Value: float64Ptr(5)},
								{Color: "red", Value: float64Ptr(10)},
							},
						},
					},
				},
			},
			{
				ID: 5, Title: "Adoption Trend", Type: "timeseries",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 8, W: 12, X: 0, Y: 6},
				Description: "Weekly adoption rate over 12 weeks",
				Targets: []dashboards.Target{
					{Expr: `aibox_active_users_7d / aibox_total_developers * 100`, LegendFmt: "adoption %", RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{Defaults: dashboards.FieldDefaults{Unit: "percent"}},
			},
			{
				ID: 6, Title: "Startup Time Trend", Type: "timeseries",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 8, W: 12, X: 12, Y: 6},
				Description: "Cold and warm start p95 over 4 weeks",
				Targets: []dashboards.Target{
					{Expr: `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="cold"}[1h]))`, LegendFmt: "cold p95", RefID: "A"},
					{Expr: `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="warm"}[1h]))`, LegendFmt: "warm p95", RefID: "B"},
				},
				FieldConfig: &dashboards.FieldConfig{Defaults: dashboards.FieldDefaults{Unit: "s"}},
			},
			{
				ID: 7, Title: "Adoption by Team", Type: "bargauge",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 8, W: 12, X: 0, Y: 14},
				Description: "Per-team adoption percentage",
				Targets: []dashboards.Target{
					{Expr: `count by (team) (aibox_sandbox_status{active_7d="true"}) / on(team) group_left aibox_team_size * 100`, LegendFmt: "{{team}}", RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{Defaults: dashboards.FieldDefaults{Unit: "percent"}},
			},
			{
				ID: 8, Title: "Security Events", Type: "timeseries",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 8, W: 12, X: 12, Y: 14},
				Description: "Security events by severity over 12 weeks",
				Targets: []dashboards.Target{
					{Expr: `sum by (severity) (increase(aibox_security_incidents_total[7d]))`, LegendFmt: "{{severity}}", RefID: "A"},
				},
			},
			{
				ID: 9, Title: "Mandatory Cutover Readiness", Type: "stat",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 4, W: 12, X: 0, Y: 22},
				Description: "Metric-gated cutover: adoption >90% (2wk), tickets <3/wk (4wk), all teams validated",
				Targets: []dashboards.Target{
					{Expr: `aibox_cutover_conditions_met`, RefID: "A"},
				},
				FieldConfig: &dashboards.FieldConfig{
					Defaults: dashboards.FieldDefaults{
						Thresholds: &dashboards.Thresholds{
							Mode: "absolute",
							Steps: []dashboards.ThresholdStep{
								{Color: "red", Value: nil},
								{Color: "yellow", Value: float64Ptr(2)},
								{Color: "green", Value: float64Ptr(3)},
							},
						},
					},
				},
			},
			{
				ID: 10, Title: "Fallback Reasons", Type: "bargauge",
				DataSource: dataSource,
				GridPos:    dashboards.GridPos{H: 4, W: 12, X: 12, Y: 22},
				Description: "Breakdown of fallback reasons over 30 days",
				Targets: []dashboards.Target{
					{Expr: `sum by (reason) (increase(aibox_fallback_events_total[30d]))`, LegendFmt: "{{reason}}", RefID: "A"},
				},
			},
		},
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
