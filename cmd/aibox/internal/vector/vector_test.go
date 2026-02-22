package vector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultVectorConfig(t *testing.T) {
	cfg := DefaultVectorConfig()

	if cfg.ConfigPath != "/etc/aibox/vector.toml" {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, "/etc/aibox/vector.toml")
	}
	if cfg.DataDir != "/var/lib/aibox/vector" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/var/lib/aibox/vector")
	}
	if cfg.BufferMaxBytes != 500*1024*1024 {
		t.Errorf("BufferMaxBytes = %d, want %d", cfg.BufferMaxBytes, 500*1024*1024)
	}
	if cfg.APIAddr != "127.0.0.1" {
		t.Errorf("APIAddr = %q, want %q", cfg.APIAddr, "127.0.0.1")
	}
	if cfg.APIPort != 8686 {
		t.Errorf("APIPort = %d, want %d", cfg.APIPort, 8686)
	}
	if cfg.SinkType != "file" {
		t.Errorf("SinkType = %q, want %q", cfg.SinkType, "file")
	}
}

func TestNewVectorManager_DefaultsFilled(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})

	if mgr.cfg.ConfigPath != "/etc/aibox/vector.toml" {
		t.Errorf("ConfigPath = %q, want default", mgr.cfg.ConfigPath)
	}
	if mgr.cfg.DataDir != "/var/lib/aibox/vector" {
		t.Errorf("DataDir = %q, want default", mgr.cfg.DataDir)
	}
	if mgr.cfg.AuditLogPath != "/var/log/aibox/audit.jsonl" {
		t.Errorf("AuditLogPath = %q, want default", mgr.cfg.AuditLogPath)
	}
	if mgr.cfg.ProxyLogPath != "/var/log/aibox/proxy-access.log" {
		t.Errorf("ProxyLogPath = %q, want default", mgr.cfg.ProxyLogPath)
	}
	if mgr.cfg.SinkType != "file" {
		t.Errorf("SinkType = %q, want default", mgr.cfg.SinkType)
	}
	if mgr.cfg.Cluster != "default" {
		t.Errorf("Cluster = %q, want default", mgr.cfg.Cluster)
	}
}

func TestNewVectorManager_CustomConfig(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{
		ConfigPath: "/custom/vector.toml",
		APIPort:    9999,
		SinkType:   "http",
		Cluster:    "production",
	})

	if mgr.cfg.ConfigPath != "/custom/vector.toml" {
		t.Errorf("ConfigPath = %q, want custom", mgr.cfg.ConfigPath)
	}
	if mgr.cfg.APIPort != 9999 {
		t.Errorf("APIPort = %d, want 9999", mgr.cfg.APIPort)
	}
	if mgr.cfg.SinkType != "http" {
		t.Errorf("SinkType = %q, want http", mgr.cfg.SinkType)
	}
	if mgr.cfg.Cluster != "production" {
		t.Errorf("Cluster = %q, want production", mgr.cfg.Cluster)
	}
	// Defaults should still be applied for zero-value fields.
	if mgr.cfg.DataDir != "/var/lib/aibox/vector" {
		t.Errorf("DataDir = %q, want default", mgr.cfg.DataDir)
	}
}

func TestGenerateConfig_HasHeader(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "AI-Box Vector Log Collection Pipeline") {
		t.Error("config should contain header comment")
	}
	if !strings.Contains(config, "DO NOT EDIT MANUALLY") {
		t.Error("config should contain DO NOT EDIT warning")
	}
}

func TestGenerateConfig_DataDir(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{DataDir: "/custom/data"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `data_dir = "/custom/data"`) {
		t.Error("config should set custom data_dir")
	}
}

func TestGenerateConfig_APISection(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{APIAddr: "0.0.0.0", APIPort: 9090})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[api]") {
		t.Error("config should have [api] section")
	}
	if !strings.Contains(config, `address = "0.0.0.0:9090"`) {
		t.Error("config should have custom API address")
	}
}

func TestGenerateConfig_AuditSource(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{AuditLogPath: "/custom/audit.jsonl"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sources.aibox_audit]") {
		t.Error("config should have aibox_audit source")
	}
	if !strings.Contains(config, `include = ["/custom/audit.jsonl"]`) {
		t.Error("config should use custom audit log path")
	}
	if !strings.Contains(config, `type = "file"`) {
		t.Error("audit source should use file type")
	}
}

func TestGenerateConfig_SquidSource(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sources.squid_proxy]") {
		t.Error("config should have squid_proxy source")
	}
	if !strings.Contains(config, "/var/log/aibox/proxy-access.log") {
		t.Error("config should include proxy log path")
	}
}

func TestGenerateConfig_OPASource(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sources.opa_decisions]") {
		t.Error("config should have opa_decisions source")
	}
	if !strings.Contains(config, "/var/log/aibox/decisions.jsonl") {
		t.Error("config should include decision log path")
	}
}

func TestGenerateConfig_JournaldSource(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sources.journald_aibox]") {
		t.Error("config should have journald source")
	}
	if !strings.Contains(config, `type = "journald"`) {
		t.Error("journald source should use journald type")
	}
	if !strings.Contains(config, "aibox-agent") {
		t.Error("journald source should include aibox-agent unit")
	}
	if !strings.Contains(config, "aibox-llm-proxy") {
		t.Error("journald source should include aibox-llm-proxy unit")
	}
}

func TestGenerateConfig_TransformParseAudit(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{Hostname: "dev-ws-01", Cluster: "eng-team"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[transforms.parse_audit]") {
		t.Error("config should have parse_audit transform")
	}
	if !strings.Contains(config, `type = "remap"`) {
		t.Error("parse_audit should use remap type")
	}
	if !strings.Contains(config, "parse_json!") {
		t.Error("parse_audit should parse JSON")
	}
	if !strings.Contains(config, `"dev-ws-01"`) {
		t.Error("parse_audit should enrich with hostname")
	}
	if !strings.Contains(config, `"eng-team"`) {
		t.Error("parse_audit should enrich with cluster")
	}
}

func TestGenerateConfig_TransformEnrichment(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{Hostname: "host1", Cluster: "prod"})
	config := mgr.GenerateConfig()

	// All transforms should add hostname, cluster, classification_level, and pipeline_ts.
	transforms := []string{"parse_audit", "parse_squid", "parse_decisions", "parse_runtime", "filter_llm", "enrich_journald"}
	for _, tr := range transforms {
		section := "[transforms." + tr + "]"
		if !strings.Contains(config, section) {
			t.Errorf("config should have %s transform", section)
		}
	}

	// pipeline_ts should be added in transforms (all except filter_llm which filters from parse_audit).
	if count := strings.Count(config, "pipeline_ts = now()"); count < 5 {
		t.Errorf("expected pipeline_ts enrichment in at least 5 transforms, found %d", count)
	}

	// classification_level should be set in transforms.
	if !strings.Contains(config, `classification_level = "standard"`) {
		t.Error("transforms should enrich with classification_level")
	}
}

func TestGenerateConfig_FileSink(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{SinkType: "file", SinkPath: "/custom/out.jsonl"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sinks.aibox_out]") {
		t.Error("config should have aibox_out sink")
	}
	if !strings.Contains(config, `type = "file"`) {
		t.Error("file sink should have file type")
	}
	if !strings.Contains(config, `path = "/custom/out.jsonl"`) {
		t.Error("file sink should use custom path")
	}
	if !strings.Contains(config, `encoding.codec = "json"`) {
		t.Error("file sink should use JSON encoding")
	}
}

func TestGenerateConfig_HTTPSink(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{
		SinkType:     "http",
		SinkEndpoint: "https://siem.internal/api/v1/ingest",
	})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `type = "http"`) {
		t.Error("http sink should have http type")
	}
	if !strings.Contains(config, `uri = "https://siem.internal/api/v1/ingest"`) {
		t.Error("http sink should have endpoint URI")
	}
	if !strings.Contains(config, `method = "post"`) {
		t.Error("http sink should use POST method")
	}
}

func TestGenerateConfig_S3Sink(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{
		SinkType:     "s3",
		SinkEndpoint: "https://minio.internal:9000",
	})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `type = "aws_s3"`) {
		t.Error("s3 sink should have aws_s3 type")
	}
	if !strings.Contains(config, `endpoint = "https://minio.internal:9000"`) {
		t.Error("s3 sink should have custom endpoint")
	}
	if !strings.Contains(config, `compression = "gzip"`) {
		t.Error("s3 sink should use gzip compression")
	}
}

func TestGenerateConfig_ConsoleSink(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{SinkType: "console"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `type = "console"`) {
		t.Error("console sink should have console type")
	}
}

func TestGenerateConfig_DiskBuffer(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{SinkType: "file"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sinks.aibox_out.buffer]") {
		t.Error("file sink should have disk buffer configuration")
	}
	if !strings.Contains(config, `type = "disk"`) {
		t.Error("buffer should be disk-backed")
	}
	if !strings.Contains(config, `when_full = "block"`) {
		t.Error("buffer should block when full (at-least-once delivery)")
	}
}

func TestGenerateConfig_ConsoleSinkNoBuffer(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{SinkType: "console"})
	config := mgr.GenerateConfig()

	if strings.Contains(config, "[sinks.aibox_out.buffer]") {
		t.Error("console sink should NOT have disk buffer")
	}
}

func TestGenerateConfig_SinkInputsIncludeAllTransforms(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	// With default RuntimeBackend=falco, sink should receive from all 6 transforms.
	expectedInputs := `inputs = ["parse_audit", "parse_squid", "parse_decisions", "parse_runtime", "filter_llm", "enrich_journald"]`
	if !strings.Contains(config, expectedInputs) {
		t.Errorf("sink should include all transform inputs, got config:\n%s", config)
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "vector.toml")

	mgr := NewVectorManager(VectorConfig{})
	if err := mgr.WriteConfig(path); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if len(data) == 0 {
		t.Error("written config should not be empty")
	}
	if !strings.Contains(string(data), "[sources.aibox_audit]") {
		t.Error("written config should contain sources")
	}
}

func TestWriteConfig_DefaultPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vector.toml")

	mgr := NewVectorManager(VectorConfig{ConfigPath: path})
	if err := mgr.WriteConfig(""); err != nil {
		t.Fatalf("WriteConfig with default: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected config file at default path")
	}
}

func TestConfigReturnsReadOnlyCopy(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{Cluster: "test-cluster"})
	cfg := mgr.Config()

	if cfg.Cluster != "test-cluster" {
		t.Errorf("Config().Cluster = %q, want %q", cfg.Cluster, "test-cluster")
	}
}

func TestGenerateConfig_AllSourcesPresent(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	sources := []string{
		"[sources.aibox_audit]",
		"[sources.squid_proxy]",
		"[sources.opa_decisions]",
		"[sources.journald_aibox]",
	}

	for _, src := range sources {
		if !strings.Contains(config, src) {
			t.Errorf("config should contain source %s", src)
		}
	}
}

func TestGenerateConfig_AllTransformsPresent(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	transforms := []string{
		"[transforms.parse_audit]",
		"[transforms.parse_squid]",
		"[transforms.parse_decisions]",
		"[transforms.enrich_journald]",
	}

	for _, tr := range transforms {
		if !strings.Contains(config, tr) {
			t.Errorf("config should contain transform %s", tr)
		}
	}
}

func TestGenerateConfig_ValidTOMLStructure(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})
	config := mgr.GenerateConfig()

	// Basic structural checks for valid TOML.
	// Ensure section headers are properly formatted.
	sections := []string{
		"[api]",
		"[sources.aibox_audit]",
		"[sources.squid_proxy]",
		"[sources.opa_decisions]",
		"[sources.journald_aibox]",
		"[transforms.parse_audit]",
		"[transforms.parse_squid]",
		"[transforms.parse_decisions]",
		"[transforms.enrich_journald]",
		"[sinks.aibox_out]",
	}

	for _, section := range sections {
		if !strings.Contains(config, section) {
			t.Errorf("config missing TOML section %s", section)
		}
	}
}

// --- RuntimeBackend tests ---

func TestGenerateConfig_RuntimeBackendFalco(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{RuntimeBackend: "falco"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[sources.falco_alerts]") {
		t.Error("falco backend should create falco_alerts source")
	}
	if strings.Contains(config, "[sources.auditd_events]") {
		t.Error("falco backend should not create auditd_events source")
	}
	if !strings.Contains(config, "[transforms.parse_runtime]") {
		t.Error("falco backend should create parse_runtime transform")
	}
	if !strings.Contains(config, `.source = "falco"`) {
		t.Error("parse_runtime should set source to falco")
	}
	// Journald should include falco unit.
	if !strings.Contains(config, `"falco"`) {
		t.Error("journald units should include falco when backend is falco")
	}
}

func TestGenerateConfig_RuntimeBackendAuditd(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{RuntimeBackend: "auditd"})
	config := mgr.GenerateConfig()

	if strings.Contains(config, "[sources.falco_alerts]") {
		t.Error("auditd backend should not create falco_alerts source")
	}
	if !strings.Contains(config, "[sources.auditd_events]") {
		t.Error("auditd backend should create auditd_events source")
	}
	if !strings.Contains(config, "[transforms.parse_runtime]") {
		t.Error("auditd backend should create parse_runtime transform")
	}
	if !strings.Contains(config, `.source = "auditd"`) {
		t.Error("parse_runtime should set source to auditd")
	}
	if !strings.Contains(config, `.event_type = "auditd.event"`) {
		t.Error("parse_runtime should set event_type for auditd")
	}
}

func TestGenerateConfig_RuntimeBackendNone(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{RuntimeBackend: "none"})
	config := mgr.GenerateConfig()

	if strings.Contains(config, "[sources.falco_alerts]") {
		t.Error("none backend should not create falco_alerts source")
	}
	if strings.Contains(config, "[sources.auditd_events]") {
		t.Error("none backend should not create auditd_events source")
	}
	if strings.Contains(config, "[transforms.parse_runtime]") {
		t.Error("none backend should not create parse_runtime transform")
	}
	// Sink inputs should not include parse_runtime.
	if strings.Contains(config, `"parse_runtime"`) {
		t.Error("sink inputs should not include parse_runtime when backend is none")
	}
	if !strings.Contains(config, `runtime_backend = "none"`) {
		t.Error("should include comment about disabled runtime backend")
	}
}

// --- LLMLoggingMode tests ---

func TestGenerateConfig_LLMLoggingModeFull(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{LLMLoggingMode: "full"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[transforms.filter_llm]") {
		t.Error("config should have filter_llm transform")
	}
	if !strings.Contains(config, "full mode: pass through") {
		t.Error("full mode should have pass-through comment")
	}
	if strings.Contains(config, "sha2(") {
		t.Error("full mode should not hash payloads")
	}
	if strings.Contains(config, "del(.details.request_body)") {
		t.Error("full mode should not delete payloads")
	}
}

func TestGenerateConfig_LLMLoggingModeHash(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{LLMLoggingMode: "hash"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[transforms.filter_llm]") {
		t.Error("config should have filter_llm transform")
	}
	if !strings.Contains(config, "sha2(string!(.details.request_body))") {
		t.Error("hash mode should hash request_body")
	}
	if !strings.Contains(config, "sha2(string!(.details.response_body))") {
		t.Error("hash mode should hash response_body")
	}
}

func TestGenerateConfig_LLMLoggingModeMetadataOnly(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{LLMLoggingMode: "metadata_only"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "[transforms.filter_llm]") {
		t.Error("config should have filter_llm transform")
	}
	if !strings.Contains(config, "del(.details.request_body)") {
		t.Error("metadata_only should delete request_body")
	}
	if !strings.Contains(config, "del(.details.response_body)") {
		t.Error("metadata_only should delete response_body")
	}
	if !strings.Contains(config, "del(.details.prompt)") {
		t.Error("metadata_only should delete prompt")
	}
	if !strings.Contains(config, "del(.details.completion)") {
		t.Error("metadata_only should delete completion")
	}
}

// --- ClassificationLevel tests ---

func TestGenerateConfig_ClassificationLevelStandard(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{ClassificationLevel: "standard"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `.classification_level = "standard"`) {
		t.Error("transforms should enrich with standard classification_level")
	}
}

func TestGenerateConfig_ClassificationLevelClassified(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{ClassificationLevel: "classified"})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, `.classification_level = "classified"`) {
		t.Error("transforms should enrich with classified classification_level")
	}
	// Every enriched transform should have it.
	count := strings.Count(config, `classification_level = "classified"`)
	if count < 5 {
		t.Errorf("expected classification_level in at least 5 transforms, found %d", count)
	}
}

// --- Default config field tests ---

func TestDefaultVectorConfig_PolicyFields(t *testing.T) {
	cfg := DefaultVectorConfig()

	if cfg.RuntimeBackend != "falco" {
		t.Errorf("RuntimeBackend = %q, want %q", cfg.RuntimeBackend, "falco")
	}
	if cfg.LLMLoggingMode != "full" {
		t.Errorf("LLMLoggingMode = %q, want %q", cfg.LLMLoggingMode, "full")
	}
	if cfg.ClassificationLevel != "standard" {
		t.Errorf("ClassificationLevel = %q, want %q", cfg.ClassificationLevel, "standard")
	}
}

func TestNewVectorManager_PolicyFieldDefaults(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{})

	if mgr.cfg.RuntimeBackend != "falco" {
		t.Errorf("RuntimeBackend = %q, want default falco", mgr.cfg.RuntimeBackend)
	}
	if mgr.cfg.LLMLoggingMode != "full" {
		t.Errorf("LLMLoggingMode = %q, want default full", mgr.cfg.LLMLoggingMode)
	}
	if mgr.cfg.ClassificationLevel != "standard" {
		t.Errorf("ClassificationLevel = %q, want default standard", mgr.cfg.ClassificationLevel)
	}
}

func TestGenerateConfig_AuditdSinkInputsExcludeFalco(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{RuntimeBackend: "auditd"})
	config := mgr.GenerateConfig()

	expectedInputs := `inputs = ["parse_audit", "parse_squid", "parse_decisions", "parse_runtime", "filter_llm", "enrich_journald"]`
	if !strings.Contains(config, expectedInputs) {
		t.Error("auditd backend should still include parse_runtime in sink inputs")
	}
}

func TestGenerateConfig_NoneSinkInputsExcludeRuntime(t *testing.T) {
	mgr := NewVectorManager(VectorConfig{RuntimeBackend: "none"})
	config := mgr.GenerateConfig()

	expectedInputs := `inputs = ["parse_audit", "parse_squid", "parse_decisions", "filter_llm", "enrich_journald"]`
	if !strings.Contains(config, expectedInputs) {
		t.Errorf("none backend should exclude parse_runtime from sink inputs")
	}
}
