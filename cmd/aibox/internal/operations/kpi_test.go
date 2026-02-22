package operations

import (
	"strings"
	"testing"
	"time"
)

func TestNewAdoptionKPI(t *testing.T) {
	kpi := NewAdoptionKPI(180, 200)
	if kpi.Value != 90.0 {
		t.Errorf("expected 90.0%%, got %.1f%%", kpi.Value)
	}
	if !kpi.Met {
		t.Error("90% adoption should meet 90% target")
	}

	kpi = NewAdoptionKPI(100, 200)
	if kpi.Value != 50.0 {
		t.Errorf("expected 50.0%%, got %.1f%%", kpi.Value)
	}
	if kpi.Met {
		t.Error("50% adoption should not meet 90% target")
	}
}

func TestNewAdoptionKPI_ZeroDevelopers(t *testing.T) {
	kpi := NewAdoptionKPI(0, 0)
	if kpi.Value != 0 {
		t.Errorf("expected 0 for zero developers, got %.1f", kpi.Value)
	}
}

func TestNewStartupKPI(t *testing.T) {
	kpi := NewStartupKPI("Cold Start p95", 85.0, 90.0)
	if !kpi.Met {
		t.Error("85s should meet 90s target")
	}

	kpi = NewStartupKPI("Cold Start p95", 95.0, 90.0)
	if kpi.Met {
		t.Error("95s should not meet 90s target")
	}
}

func TestNewTicketKPI(t *testing.T) {
	kpi := NewTicketKPI(2)
	if !kpi.Met {
		t.Error("2 tickets should meet <3 target")
	}

	kpi = NewTicketKPI(5)
	if kpi.Met {
		t.Error("5 tickets should not meet <3 target")
	}

	kpi = NewTicketKPI(3)
	if !kpi.Met {
		t.Error("3 tickets should meet <=3 target")
	}
}

func TestNewFallbackKPI(t *testing.T) {
	kpi := NewFallbackKPI(5, 100)
	if kpi.Value != 5.0 {
		t.Errorf("expected 5.0%%, got %.1f%%", kpi.Value)
	}
	if !kpi.Met {
		t.Error("5% should meet <=5% target")
	}

	kpi = NewFallbackKPI(10, 100)
	if kpi.Met {
		t.Error("10% should not meet <=5% target")
	}
}

func TestNewFallbackKPI_ZeroStarts(t *testing.T) {
	kpi := NewFallbackKPI(0, 0)
	if !kpi.Met {
		t.Error("zero starts should be considered as target met")
	}
}

func TestNewSecurityEventsKPI(t *testing.T) {
	kpi := NewSecurityEventsKPI(3)
	if kpi.Value != 3.0 {
		t.Errorf("expected 3, got %.0f", kpi.Value)
	}
	if kpi.Name != "Security Events (7d)" {
		t.Errorf("unexpected name: %s", kpi.Name)
	}
}

func TestCheckCutoverReadiness_AllMet(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{85.0, 88.0, 91.0, 92.0} // last 2 >= 90
	ticketWeeks := []int{2, 2, 1, 3, 2, 1}              // last 4 <= 3
	allTeamsValidated := true

	ready, status := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if !ready {
		t.Errorf("expected ready, got not ready: %s", status)
	}
	if !strings.Contains(status, "3/3") {
		t.Errorf("expected 3/3 conditions, got: %s", status)
	}
}

func TestCheckCutoverReadiness_AdoptionNotMet(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{85.0, 88.0} // neither >= 90
	ticketWeeks := []int{2, 1, 2, 1}       // all <= 3
	allTeamsValidated := true

	ready, status := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if ready {
		t.Error("expected not ready when adoption below target")
	}
	if !strings.Contains(status, "2/3") {
		t.Errorf("expected 2/3 conditions, got: %s", status)
	}
}

func TestCheckCutoverReadiness_TicketsNotMet(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{91.0, 92.0}
	ticketWeeks := []int{2, 5, 2, 1} // week 2 exceeds target
	allTeamsValidated := true

	ready, status := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if ready {
		t.Error("expected not ready when tickets exceed target")
	}
	if !strings.Contains(status, "tickets NOT MET") {
		t.Errorf("expected tickets NOT MET in status, got: %s", status)
	}
}

func TestCheckCutoverReadiness_TeamsNotValidated(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{91.0, 92.0}
	ticketWeeks := []int{2, 1, 2, 1}
	allTeamsValidated := false

	ready, status := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if ready {
		t.Error("expected not ready when teams not validated")
	}
	if !strings.Contains(status, "teams NOT MET") {
		t.Errorf("expected teams NOT MET in status, got: %s", status)
	}
}

func TestCheckCutoverReadiness_InsufficientData(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{95.0} // only 1 week, need 2
	ticketWeeks := []int{1}          // only 1 week, need 4
	allTeamsValidated := true

	ready, _ := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if ready {
		t.Error("expected not ready with insufficient data")
	}
}

func TestCheckCutoverReadiness_NoneMet(t *testing.T) {
	cfg := DefaultKPIConfig()
	adoptionWeeks := []float64{50.0, 60.0}
	ticketWeeks := []int{10, 8, 12, 9}
	allTeamsValidated := false

	ready, status := CheckCutoverReadiness(adoptionWeeks, ticketWeeks, allTeamsValidated, cfg)
	if ready {
		t.Error("expected not ready when nothing is met")
	}
	if !strings.Contains(status, "0/3") {
		t.Errorf("expected 0/3 conditions, got: %s", status)
	}
}

func TestWeeklyReport(t *testing.T) {
	snap := KPISnapshot{
		Timestamp:      time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
		AdoptionRate:   KPI{Value: 85.5, Target: 90.0},
		ColdStartP95:   KPI{Value: 72.3, Target: 90.0},
		WarmStartP95:   KPI{Value: 12.1, Target: 15.0},
		SupportTickets: KPI{Value: 2, Target: 3},
		FallbackRate:   KPI{Value: 3.2, Target: 5.0},
		SecurityEvents: KPI{Value: 1},
		CutoverReady:   false,
		CutoverReadyText: "2/3 conditions met",
	}

	report := WeeklyReport(snap)

	if !strings.Contains(report, "2026-02-21") {
		t.Error("report should contain date")
	}
	if !strings.Contains(report, "85.5%") {
		t.Error("report should contain adoption rate")
	}
	if !strings.Contains(report, "72.3s") {
		t.Error("report should contain cold start time")
	}
	if !strings.Contains(report, "NOT READY") {
		t.Error("report should show NOT READY")
	}
}

func TestWeeklyReport_Ready(t *testing.T) {
	snap := KPISnapshot{
		Timestamp:        time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		AdoptionRate:     KPI{Value: 93.0, Target: 90.0},
		ColdStartP95:     KPI{Value: 65.0, Target: 90.0},
		WarmStartP95:     KPI{Value: 10.0, Target: 15.0},
		SupportTickets:   KPI{Value: 1, Target: 3},
		FallbackRate:     KPI{Value: 2.0, Target: 5.0},
		SecurityEvents:   KPI{Value: 0},
		CutoverReady:     true,
		CutoverReadyText: "3/3 conditions met",
	}

	report := WeeklyReport(snap)
	if !strings.Contains(report, "READY") {
		t.Error("report should show READY")
	}
	// Should not contain "NOT READY" -- check exact match
	if strings.Contains(report, "NOT READY") {
		t.Error("report should not show NOT READY when ready")
	}
}

func TestDefaultKPIConfig(t *testing.T) {
	cfg := DefaultKPIConfig()
	if cfg.TotalDevelopers != 200 {
		t.Errorf("TotalDevelopers: expected 200, got %d", cfg.TotalDevelopers)
	}
	if cfg.AdoptionTarget != 90.0 {
		t.Errorf("AdoptionTarget: expected 90.0, got %.1f", cfg.AdoptionTarget)
	}
	if cfg.ColdStartTarget != 90.0 {
		t.Errorf("ColdStartTarget: expected 90.0, got %.1f", cfg.ColdStartTarget)
	}
	if cfg.WarmStartTarget != 15.0 {
		t.Errorf("WarmStartTarget: expected 15.0, got %.1f", cfg.WarmStartTarget)
	}
	if cfg.TicketTarget != 3.0 {
		t.Errorf("TicketTarget: expected 3.0, got %.1f", cfg.TicketTarget)
	}
	if cfg.FallbackTarget != 5.0 {
		t.Errorf("FallbackTarget: expected 5.0, got %.1f", cfg.FallbackTarget)
	}
}

func TestKPIDashboard(t *testing.T) {
	dash := KPIDashboard("")
	if dash.UID != "aibox-kpi-tracking" {
		t.Errorf("unexpected UID: %s", dash.UID)
	}
	if len(dash.Panels) != 10 {
		t.Errorf("expected 10 panels, got %d", len(dash.Panels))
	}

	// Verify key panels exist.
	panelTitles := make(map[string]bool)
	for _, p := range dash.Panels {
		panelTitles[p.Title] = true
	}

	required := []string{
		"Adoption Rate (7d)",
		"Cold Start p95",
		"Support Threads (7d)",
		"Fallback Rate (7d)",
		"Mandatory Cutover Readiness",
	}
	for _, title := range required {
		if !panelTitles[title] {
			t.Errorf("missing required panel: %s", title)
		}
	}
}

func TestKPIDashboard_CustomDataSource(t *testing.T) {
	dash := KPIDashboard("Custom Source")
	for _, p := range dash.Panels {
		if p.DataSource != "Custom Source" {
			t.Errorf("panel %q has datasource %q, expected 'Custom Source'", p.Title, p.DataSource)
		}
	}
}
