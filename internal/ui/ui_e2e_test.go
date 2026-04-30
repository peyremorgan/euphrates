package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"euphrates/internal/state"
)

func TestJoinCompletionFlow_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	// Simulate an existing joined channel and a prefetched LIST cache.
	u.state.JoinChannel("#go")
	u.state.SetChannelListCache([]string{"#go", "#golang", "#gophers"})

	// First tab on an empty /join argument inserts '#'.
	u.input.SetText("/join ")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed at /join prompt")
	}
	if got := u.input.GetText(); got != "/join #" {
		t.Fatalf("input=%q want /join #", got)
	}

	// Tab-complete using prefetched LIST and exclusion of already joined #go.
	u.input.SetText("/join #gola")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed at partial")
	}
	if got := u.input.GetText(); got != "/join #golang" {
		t.Fatalf("input=%q want /join #golang", got)
	}

	// Submit /join command end-to-end through input handler.
	u.onInputDone(tcell.KeyEnter)
	if len(fs.joins) != 1 || fs.joins[0] != "#golang" {
		t.Fatalf("joins=%v", fs.joins)
	}
}
