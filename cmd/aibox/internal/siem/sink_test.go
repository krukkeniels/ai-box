package siem

import (
	"strings"
	"testing"
)

func TestDefaultSinkConfig(t *testing.T) {
	cfg := DefaultSinkConfig()

	if cfg.Type != SinkHTTP {
		t.Errorf("Type = %q, want %q", cfg.Type, SinkHTTP)
	}
	if cfg.Format != "json" {
		t.Errorf("Format = %q, want %q", cfg.Format, "json")
	}
	if !cfg.TLS {
		t.Error("TLS should be true by default")
	}
}

func TestGenerateVectorSink_HTTP(t *testing.T) {
	cfg := SinkConfig{
		Type:     SinkHTTP,
		Endpoint: "https://siem.internal/api/v1/ingest",
		Format:   "json",
		TLS:      true,
	}
	config := GenerateVectorSink(cfg)

	checks := []struct {
		name string
		want string
	}{
		{"header", "AI-Box SIEM Integration Sink"},
		{"sink section", "[sinks.siem]"},
		{"type", `type = "http"`},
		{"method", `method = "post"`},
		{"uri", `uri = "https://siem.internal/api/v1/ingest"`},
		{"encoding", `encoding.codec = "json"`},
		{"tls", "tls.verify_certificate = true"},
		{"buffer", "[sinks.siem.buffer]"},
		{"disk buffer", `type = "disk"`},
		{"retry", "retry_max_duration_secs"},
		{"inputs", "parse_audit"},
	}

	for _, tt := range checks {
		if !strings.Contains(config, tt.want) {
			t.Errorf("HTTP sink missing %s: %q", tt.name, tt.want)
		}
	}
}

func TestGenerateVectorSink_Syslog(t *testing.T) {
	cfg := SinkConfig{
		Type:     SinkSyslog,
		Endpoint: "siem.internal:514",
		TLS:      true,
	}
	config := GenerateVectorSink(cfg)

	if !strings.Contains(config, `type = "socket"`) {
		t.Error("syslog sink should use socket type")
	}
	if !strings.Contains(config, `mode = "tcp"`) {
		t.Error("syslog sink should use TCP mode")
	}
	if !strings.Contains(config, `address = "siem.internal:514"`) {
		t.Error("syslog sink should have correct address")
	}
	if !strings.Contains(config, "tls.enabled = true") {
		t.Error("syslog sink should enable TLS")
	}
}

func TestGenerateVectorSink_S3(t *testing.T) {
	cfg := SinkConfig{
		Type:     SinkS3,
		Endpoint: "https://minio.internal:9000",
		Format:   "json",
	}
	config := GenerateVectorSink(cfg)

	if !strings.Contains(config, `type = "aws_s3"`) {
		t.Error("S3 sink should use aws_s3 type")
	}
	if !strings.Contains(config, `endpoint = "https://minio.internal:9000"`) {
		t.Error("S3 sink should have endpoint")
	}
	if !strings.Contains(config, `compression = "gzip"`) {
		t.Error("S3 sink should use gzip compression")
	}
	if !strings.Contains(config, `bucket = "aibox-siem"`) {
		t.Error("S3 sink should have bucket name")
	}
}

func TestGenerateVectorSink_Kafka(t *testing.T) {
	cfg := SinkConfig{
		Type:     SinkKafka,
		Endpoint: "kafka.internal:9092",
		Format:   "json",
		TLS:      true,
	}
	config := GenerateVectorSink(cfg)

	if !strings.Contains(config, `type = "kafka"`) {
		t.Error("Kafka sink should use kafka type")
	}
	if !strings.Contains(config, `bootstrap_servers = "kafka.internal:9092"`) {
		t.Error("Kafka sink should have bootstrap servers")
	}
	if !strings.Contains(config, `topic = "aibox-audit-events"`) {
		t.Error("Kafka sink should have topic")
	}
	if !strings.Contains(config, "tls.enabled = true") {
		t.Error("Kafka sink should enable TLS")
	}
}

func TestGenerateVectorSink_AllSinksHaveInputs(t *testing.T) {
	sinkTypes := []SinkType{SinkSyslog, SinkHTTP, SinkS3, SinkKafka}
	expectedInputs := `inputs = ["parse_audit", "parse_squid", "parse_decisions", "enrich_journald"]`

	for _, st := range sinkTypes {
		cfg := SinkConfig{Type: st, Endpoint: "test:1234"}
		config := GenerateVectorSink(cfg)

		if !strings.Contains(config, expectedInputs) {
			t.Errorf("%s sink missing standard inputs", st)
		}
	}
}

func TestGenerateVectorSink_AllSinksHaveHeader(t *testing.T) {
	sinkTypes := []SinkType{SinkSyslog, SinkHTTP, SinkS3, SinkKafka}

	for _, st := range sinkTypes {
		cfg := SinkConfig{Type: st, Endpoint: "test:1234"}
		config := GenerateVectorSink(cfg)

		if !strings.Contains(config, "DO NOT EDIT MANUALLY") {
			t.Errorf("%s sink missing header", st)
		}
	}
}

func TestGenerateVectorSink_DiskBackedBufferForPersistentSinks(t *testing.T) {
	persistentSinks := []SinkType{SinkHTTP, SinkS3, SinkKafka}

	for _, st := range persistentSinks {
		cfg := SinkConfig{Type: st, Endpoint: "test:1234", Format: "json"}
		config := GenerateVectorSink(cfg)

		if !strings.Contains(config, "[sinks.siem.buffer]") {
			t.Errorf("%s sink should have disk buffer", st)
		}
		if !strings.Contains(config, `type = "disk"`) {
			t.Errorf("%s sink buffer should be disk-backed", st)
		}
	}
}

func TestGenerateVectorSink_HTTPNoTLS(t *testing.T) {
	cfg := SinkConfig{
		Type:     SinkHTTP,
		Endpoint: "http://siem.internal/ingest",
		Format:   "json",
		TLS:      false,
	}
	config := GenerateVectorSink(cfg)

	if strings.Contains(config, "tls.verify_certificate") {
		t.Error("HTTP sink without TLS should not have TLS config")
	}
}
