package container

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProbeSSH_NoListener(t *testing.T) {
	// Port 59999 should have no listener — probe must fail quickly.
	result := ProbeSSH("127.0.0.1", 59999, 1*time.Second)
	if result.Ready {
		t.Fatal("expected not ready when no listener is present")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestProbeSSH_TCPOnlyNoSSH(t *testing.T) {
	// Start a TCP listener that accepts connections but never sends a banner.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Accept and immediately close connections.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	result := ProbeSSH("127.0.0.1", port, 2*time.Second)
	if result.Ready {
		t.Fatal("expected not ready when listener sends no SSH banner")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestProbeSSH_ValidSSHBanner(t *testing.T) {
	// Start a TCP listener that sends a valid SSH-2.0 banner.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			fmt.Fprintf(conn, "SSH-2.0-OpenSSH_9.6\r\n")
			conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	result := ProbeSSH("127.0.0.1", port, 5*time.Second)
	if !result.Ready {
		t.Fatalf("expected ready, got error: %s", result.Error)
	}
	if result.Banner != "SSH-2.0-OpenSSH_9.6" {
		t.Fatalf("expected banner SSH-2.0-OpenSSH_9.6, got %q", result.Banner)
	}
}

func TestProbeSSHResult_String(t *testing.T) {
	tests := []struct {
		name   string
		result SSHProbeResult
		want   string
	}{
		{
			name:   "ready",
			result: SSHProbeResult{Ready: true, Banner: "SSH-2.0-OpenSSH_9.6"},
			want:   "SSH ready (SSH-2.0-OpenSSH_9.6)",
		},
		{
			name:   "not ready",
			result: SSHProbeResult{Ready: false, Error: "connection refused"},
			want:   "SSH not ready: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIDEHint_WSL(t *testing.T) {
	hint := ideHint(true, 2222)
	if strings.Contains(hint, "'Remote-SSH: Connect to Host...'") {
		t.Error("WSL hint should not use generic Remote-SSH instructions")
	}
	if !strings.Contains(hint, "code .") {
		t.Error("WSL hint should suggest 'code .' from WSL terminal")
	}
}

func TestIDEHint_Linux(t *testing.T) {
	hint := ideHint(false, 2222)
	if !strings.Contains(hint, "Remote-SSH") {
		t.Error("Linux hint should mention Remote-SSH")
	}
}
