package irc

import "testing"

func TestClientE2E_InProcessServer(t *testing.T) {
	srv, addr := startMiniIRCServer(t)
	defer srv.close()

	runTwoClientScenario(t, addr)
}
