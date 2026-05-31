package ui

import (
	"strings"
	"testing"
)

func TestChannelBrowser_SetChannelsAndJoinedIndicator(t *testing.T) {
	b := newChannelBrowser()
	b.setChannels([]string{"#alpha", "#beta"}, func(name string) bool {
		return strings.EqualFold(name, "#beta")
	})

	main0, _ := b.listView.GetItemText(0)
	main1, _ := b.listView.GetItemText(1)
	if main0 != "  #alpha" {
		t.Fatalf("item0=%q", main0)
	}
	if main1 != "* #beta" {
		t.Fatalf("item1=%q", main1)
	}
}

func TestChannelBrowser_AppendRuneResetsSelectionOnGrowth(t *testing.T) {
	b := newChannelBrowser()
	b.setChannels([]string{"#archive", "#beta", "#gamma"}, nil)
	b.listView.SetCurrentItem(2)

	b.appendRune('a')
	if got := b.listView.GetCurrentItem(); got != 0 {
		t.Fatalf("selection=%d want 0", got)
	}
}

func TestChannelBrowser_BackspaceKeepsSelectionInRange(t *testing.T) {
	b := newChannelBrowser()
	b.setChannels([]string{"#archive", "#beta", "#gamma"}, nil)
	b.appendRune('a')
	b.listView.SetCurrentItem(1)

	b.backspace()
	if got := b.listView.GetCurrentItem(); got != 1 {
		t.Fatalf("selection=%d want 1", got)
	}
}

func TestChannelBrowser_StatusRight(t *testing.T) {
	b := newChannelBrowser()
	b.setChannels([]string{"#a", "#b", "#c", "#d"}, nil)
	b.listView.SetCurrentItem(2)
	b.refreshStatus()

	if got := b.statusRight.GetText(true); got != "3/4    75%" {
		t.Fatalf("status=%q", got)
	}
}
