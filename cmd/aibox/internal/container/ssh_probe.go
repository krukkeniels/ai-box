package container

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

// SSHProbeResult describes the outcome of an SSH readiness probe.
type SSHProbeResult struct {
	Ready  bool
	Banner string
	Error  string
}

func (r SSHProbeResult) String() string {
	if r.Ready {
		return fmt.Sprintf("SSH ready (%s)", r.Banner)
	}
	return fmt.Sprintf("SSH not ready: %s", r.Error)
}

// ProbeSSH attempts a TCP connection and reads the SSH banner.
// It retries with backoff up to the given timeout.
// A successful probe means the remote sent an "SSH-2.0-" banner line.
func ProbeSSH(host string, port int, timeout time.Duration) SSHProbeResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	deadline := time.Now().Add(timeout)
	delay := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			time.Sleep(delay)
			delay = min(delay*2, time.Second)
			continue
		}

		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			banner := strings.TrimSpace(scanner.Text())
			conn.Close()
			if strings.HasPrefix(banner, "SSH-2.0-") {
				return SSHProbeResult{Ready: true, Banner: banner}
			}
			return SSHProbeResult{
				Ready: false,
				Error: fmt.Sprintf("unexpected banner: %q", banner),
			}
		}

		conn.Close()
		time.Sleep(delay)
		delay = min(delay*2, time.Second)
	}

	return SSHProbeResult{
		Ready: false,
		Error: fmt.Sprintf("timeout waiting for SSH banner on %s after %s", addr, timeout),
	}
}
