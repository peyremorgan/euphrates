//go:build integration

package irc

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestClientIntegration_LiveServer(t *testing.T) {
	addr := os.Getenv("IRC_TEST_ADDR")
	if addr == "" {
		addr = "irc:6667"
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("live IRC integration skipped, server unavailable at %s: %v", addr, err)
		return
	}
	_ = conn.Close()
	runTwoClientScenario(t, addr)
}
