package state

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func newTestState() *State {
	return New(Config{MessageCap: 100, EventCap: 5})
}

func TestState_DefaultsApplied(t *testing.T) {
	s := New(Config{})
	if s.messages.Cap() != defaultMessageCap {
		t.Errorf("messageCap=%d", s.messages.Cap())
	}
	if s.events.Cap() != defaultEventCap {
		t.Errorf("eventCap=%d", s.events.Cap())
	}
}

func TestState_ServerName(t *testing.T) {
	s := New(Config{ServerName: "irc.example.org"})
	if got := s.ServerName(); got != "irc.example.org" {
		t.Errorf("serverName=%q", got)
	}
}

func TestEnsureChannel_NormalAssignsLeastPopulatedGroup(t *testing.T) {
	s := newTestState()
	c1 := s.EnsureChannel("#a")
	c2 := s.EnsureChannel("#b")
	c3 := s.EnsureChannel("#c")
	// All start at count 0, so they should get groups 0, 1, 2 in order.
	if c1.Group != GroupID(0) || c2.Group != GroupID(1) || c3.Group != GroupID(2) {
		t.Errorf("groups: %d %d %d", c1.Group, c2.Group, c3.Group)
	}
	if got := s.NumericGroupCount(GroupID(0)); got != 1 {
		t.Errorf("count(group0)=%d", got)
	}
	if got := s.NumericGroupCount(GroupID(1)); got != 1 {
		t.Errorf("count(group1)=%d", got)
	}
	if got := s.NumericGroupCount(GroupServer); got != 0 {
		t.Errorf("count(server)=%d", got)
	}
}

func TestEnsureChannel_QueryAndServer(t *testing.T) {
	s := newTestState()
	q := s.EnsureChannel("alice")
	if q.Kind != ChanQuery || q.Group != GroupQueries {
		t.Errorf("query: kind=%v group=%v", q.Kind, q.Group)
	}
	srv := s.EnsureChannel(ServerChannelName)
	if srv.Kind != ChanServer || srv.Group != GroupServer {
		t.Errorf("server: kind=%v group=%v", srv.Kind, srv.Group)
	}
}

func TestEnsureChannel_Idempotent(t *testing.T) {
	s := newTestState()
	c1 := s.EnsureChannel("#a")
	c2 := s.EnsureChannel("#a")
	if c1 != c2 {
		t.Error("EnsureChannel returned different pointers for same name")
	}
	if got := len(s.Channels()); got != 1 {
		t.Errorf("channel count=%d", got)
	}
}

func TestNormalChannelCount_ExcludesQueriesAndServer(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	s.EnsureChannel("#b")
	s.EnsureChannel("alice")
	s.EnsureChannel(ServerChannelName)

	if got := s.NormalChannelCount(); got != 2 {
		t.Errorf("normalChannelCount=%d want 2", got)
	}

	s.PartChannel("#a")
	if got := s.NormalChannelCount(); got != 1 {
		t.Errorf("after part normalChannelCount=%d want 1", got)
	}
}

func TestEnsureChannel_CaseInsensitive(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#FoO")
	if got := len(s.Channels()); got != 1 {
		t.Fatalf("channel count=%d", got)
	}
	s.EnsureChannel("#foo")
	if got := len(s.Channels()); got != 1 {
		t.Errorf("case-insensitive dedup failed: %d", got)
	}
}

func TestPartChannel_ReleasesGroupSlot(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a") // group 0
	s.EnsureChannel("#b") // group 1
	s.PartChannel("#a")
	c := s.EnsureChannel("#c")
	// group 0 should be the least-populated again
	if c.Group != GroupID(0) {
		t.Errorf("after part+rejoin, group=%d want 0", c.Group)
	}
}

func TestPartChannel_FallsBackTarget(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	s.EnsureChannel("#b")
	if s.Target() != "#a" {
		t.Fatalf("initial target=%q", s.Target())
	}
	s.PartChannel("#a")
	if s.Target() != "#b" {
		t.Errorf("after part, target=%q", s.Target())
	}
	s.PartChannel("#b")
	if s.Target() != "" {
		t.Errorf("after parting all, target=%q", s.Target())
	}
}

func TestPartChannel_UnknownIsNoop(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	s.PartChannel("#zzz")
	if got := len(s.Channels()); got != 1 {
		t.Errorf("channels=%d", got)
	}
}

func TestAppendMessage_ReturnsVisibility(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a") // visible by default
	_, vis := s.AppendMessage(Message{Channel: "#a", Nick: "x", Text: "hi", Kind: KindPrivmsg})
	if !vis {
		t.Error("expected visible")
	}
	c, _ := s.Channel("#a")
	s.SetVisible(c.Group, false)
	_, vis = s.AppendMessage(Message{Channel: "#a", Nick: "x", Text: "hi2", Kind: KindPrivmsg})
	if vis {
		t.Error("expected hidden")
	}
}

func TestAppendMessage_AutoCreatesQuery(t *testing.T) {
	s := newTestState()
	line, vis := s.AppendMessage(Message{Channel: "alice", Nick: "alice", Text: "hi", Kind: KindPrivmsg})
	if !vis {
		t.Error("expected visible")
	}
	if !strings.Contains(line, "[@alice[]") {
		t.Errorf("query format: %q", line)
	}
	c, ok := s.Channel("alice")
	if !ok || c.Kind != ChanQuery {
		t.Errorf("channel record missing or wrong kind")
	}
}

func TestRenderVisible_FiltersByGroup(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a") // group 0
	s.EnsureChannel("#b") // group 1
	s.AppendMessage(Message{Channel: "#a", Nick: "u", Text: "A1", Kind: KindPrivmsg})
	s.AppendMessage(Message{Channel: "#b", Nick: "u", Text: "B1", Kind: KindPrivmsg})
	s.AppendMessage(Message{Channel: "#a", Nick: "u", Text: "A2", Kind: KindPrivmsg})

	all := s.RenderVisible()
	if len(all) != 3 {
		t.Errorf("all: %d lines", len(all))
	}

	s.SetVisible(GroupID(1), false) // hide #b
	got := s.RenderVisible()
	if len(got) != 2 {
		t.Fatalf("after hide #b: %d lines", len(got))
	}
	for _, l := range got {
		if strings.Contains(l, "[#b[]") {
			t.Errorf("hidden #b leaked: %q", l)
		}
	}
}

func TestToggleGroup(t *testing.T) {
	s := newTestState()
	g := GroupID(3)
	if !s.IsVisible(g) {
		t.Fatal("default should be visible")
	}
	if got := s.ToggleGroup(g); got {
		t.Error("first toggle should return false")
	}
	if got := s.ToggleGroup(g); !got {
		t.Error("second toggle should return true")
	}
}

func TestSoloNumericGroup_OnlyTargetVisible(t *testing.T) {
	s := newTestState()
	s.SoloNumericGroup(GroupID(3))
	for i := 0; i < NumGroups; i++ {
		want := i == 3
		if got := s.IsVisible(GroupID(i)); got != want {
			t.Fatalf("group %d visible=%v want %v", i, got, want)
		}
	}
}

func TestSoloNumericGroup_SoloToSolo(t *testing.T) {
	s := newTestState()
	s.SoloNumericGroup(GroupID(1))
	s.SoloNumericGroup(GroupID(4))
	for i := 0; i < NumGroups; i++ {
		want := i == 4
		if got := s.IsVisible(GroupID(i)); got != want {
			t.Fatalf("group %d visible=%v want %v", i, got, want)
		}
	}
}

func TestSoloNumericGroup_PreservesSpecialGroups(t *testing.T) {
	s := newTestState()
	s.SetVisible(GroupServer, false)
	s.SetVisible(GroupQueries, true)

	s.SoloNumericGroup(GroupID(0))

	if s.IsVisible(GroupServer) {
		t.Error("server visibility should be preserved")
	}
	if !s.IsVisible(GroupQueries) {
		t.Error("queries visibility should be preserved")
	}
}

func TestSoloNumericGroup_NonNumericNoop(t *testing.T) {
	s := newTestState()
	before := s.VisibleGroups()
	s.SoloNumericGroup(GroupServer)
	after := s.VisibleGroups()
	if len(after) != len(before) {
		t.Fatalf("visibility map size changed: before=%d after=%d", len(before), len(after))
	}
	for g, v := range before {
		if after[g] != v {
			t.Fatalf("visibility for group %d changed: before=%v after=%v", g, v, after[g])
		}
	}
}

func TestNextPrevChannel_CyclesAndSkipsServer(t *testing.T) {
	s := newTestState()
	s.EnsureChannel(ServerChannelName)
	s.EnsureChannel("#a")
	s.EnsureChannel("alice")
	s.EnsureChannel("#b")
	// Initial target = first non-server = #a
	if s.Target() != "#a" {
		t.Fatalf("target=%q", s.Target())
	}
	// next: #a -> alice -> #b -> #a
	if got := s.NextChannel(); got != "alice" {
		t.Errorf("next=%q", got)
	}
	if got := s.NextChannel(); got != "#b" {
		t.Errorf("next=%q", got)
	}
	if got := s.NextChannel(); got != "#a" {
		t.Errorf("wrap=%q", got)
	}
	if got := s.PrevChannel(); got != "#b" {
		t.Errorf("prev wrap=%q", got)
	}
}

func TestNextChannel_NoSendable(t *testing.T) {
	s := newTestState()
	s.EnsureChannel(ServerChannelName)
	if got := s.NextChannel(); got != "" {
		t.Errorf("next on server-only=%q", got)
	}
}

func TestSetTarget_RejectsServer(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	s.EnsureChannel(ServerChannelName)
	s.SetTarget(ServerChannelName)
	if s.Target() == canonicalKey(ServerChannelName) {
		t.Error("server became target")
	}
}

func TestSetTarget_UnknownIsNoop(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	prev := s.Target()
	s.SetTarget("#zzz")
	if s.Target() != prev {
		t.Error("unknown target accepted")
	}
}

func TestForceTargetVisible(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	c, _ := s.Channel("#a")
	s.SetVisible(c.Group, false)
	if g := s.ForceTargetVisible(); g != c.Group {
		t.Errorf("returned group=%d", g)
	}
	if !s.IsVisible(c.Group) {
		t.Error("group not made visible")
	}
}

func TestForceTargetVisible_NoTarget(t *testing.T) {
	s := newTestState()
	if g := s.ForceTargetVisible(); g != 0 {
		t.Errorf("no-target returned %d", g)
	}
}

func TestPromptLabel_Visible(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	label := s.PromptLabel()
	if !strings.Contains(label, "[#a[]") {
		t.Errorf("label=%q", label)
	}
	if strings.HasPrefix(label, dimColor) {
		t.Errorf("visible target shouldn't be dim: %q", label)
	}
	if !strings.HasSuffix(label, " ") {
		t.Errorf("label must end with space: %q", label)
	}
}

func TestPromptLabel_HiddenIsDim(t *testing.T) {
	s := newTestState()
	s.EnsureChannel("#a")
	c, _ := s.Channel("#a")
	s.SetVisible(c.Group, false)
	label := s.PromptLabel()
	if !strings.HasPrefix(label, dimColor) {
		t.Errorf("hidden target should start with dim: %q", label)
	}
	if !strings.Contains(label, "[#a[]") {
		t.Errorf("label still must contain channel: %q", label)
	}
}

func TestPromptLabel_NoTarget(t *testing.T) {
	s := newTestState()
	if got := s.PromptLabel(); got != "" {
		t.Errorf("no-target label=%q", got)
	}
}

func TestEvents_RingedAndSnapshotted(t *testing.T) {
	s := New(Config{EventCap: 3, MessageCap: 10})
	for _, l := range []string{"e1", "e2", "e3", "e4"} {
		s.AddEvent(l)
	}
	got := s.Events()
	want := []string{"e2", "e3", "e4"}
	if len(got) != len(want) {
		t.Fatalf("events=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("events[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestSetChannelListCache_DeduplicatesCaseInsensitive(t *testing.T) {
	s := newTestState()
	s.SetChannelListCache([]string{"#Go", "#go", "#rust"})

	got := s.MatchJoinableChannels("#")
	if len(got) != 2 {
		t.Fatalf("match len=%d want 2 (%v)", len(got), got)
	}
}

func TestMatchJoinableChannels_UsesJoinedAsExclusionFilter(t *testing.T) {
	s := newTestState()
	s.SetChannelListCache([]string{"#alpha", "#beta", "#gamma"})
	s.JoinChannel("#beta")

	got := s.MatchJoinableChannels("#")
	if len(got) != 2 {
		t.Fatalf("match len=%d want 2 (%v)", len(got), got)
	}
	for _, name := range got {
		if strings.EqualFold(name, "#beta") {
			t.Fatalf("joined channel leaked into completions: %v", got)
		}
	}
}

func TestMatchJoinableChannels_PrefixAndSort(t *testing.T) {
	s := newTestState()
	s.SetChannelListCache([]string{"#Zoo", "#alpha", "#apple", "#beta"})

	got := s.MatchJoinableChannels("#a")
	want := []string{"#alpha", "#apple"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches=%v want %v", got, want)
	}
}

func TestServerNotTargetedOnFirstJoin(t *testing.T) {
	s := newTestState()
	s.EnsureChannel(ServerChannelName)
	if s.Target() != "" {
		t.Errorf("target after server-only=%q", s.Target())
	}
	s.EnsureChannel("#a")
	if s.Target() != "#a" {
		t.Errorf("target after #a=%q", s.Target())
	}
}

func TestState_ConcurrentAccess(t *testing.T) {
	s := New(Config{MessageCap: 1000, EventCap: 10})
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch := "#x"
			s.EnsureChannel(ch)
			for i := 0; i < 200; i++ {
				s.AppendMessage(Message{Channel: ch, Nick: "u", Text: "t", Kind: KindPrivmsg})
				s.AddEvent("ev")
				_ = s.RenderVisible()
				_ = s.PromptLabel()
				if i%17 == 0 {
					s.ToggleGroup(GroupID(0))
				}
			}
		}(w)
	}
	wg.Wait()
}

func TestPromptLabel_StableAcrossPalette(t *testing.T) {
	// Ensure dim swap doesn't accidentally drop the trailing reset+space.
	s := newTestState()
	s.EnsureChannel("#someChannelName")
	c, _ := s.Channel("#someChannelName")
	s.SetVisible(c.Group, false)
	label := s.PromptLabel()
	if !strings.HasSuffix(label, "[-] ") {
		t.Errorf("dim label doesn't end with reset+space: %q", label)
	}
}

func TestCustomGroupingStrategy_ReshufflesOnJoin(t *testing.T) {
	strategy := groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
		assignment := make(Assignment, len(input.Channels))
		for _, channel := range input.Channels {
			assignment[channel.Name] = 0
		}
		if len(input.Channels) >= 2 {
			assignment[input.ChangedChannel] = 9
		}
		return assignment, true, nil
	})

	s := New(Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	s.JoinChannel("#b")

	a, _ := s.Channel("#a")
	b, _ := s.Channel("#b")
	if a.Group != 0 {
		t.Fatalf("#a group=%d want 0", a.Group)
	}
	if b.Group != 9 {
		t.Fatalf("#b group=%d want 9", b.Group)
	}
	if got := s.NumericGroupCount(0); got != 1 {
		t.Fatalf("group 0 count=%d want 1", got)
	}
	if got := s.NumericGroupCount(9); got != 1 {
		t.Fatalf("group 9 count=%d want 1", got)
	}
}

func TestCustomGroupingStrategy_ReshufflesOnPart(t *testing.T) {
	strategy := groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
		assignment := make(Assignment, len(input.Channels))
		group := GroupID(0)
		if input.Trigger == GroupingTriggerPart {
			group = 7
		}
		for _, channel := range input.Channels {
			assignment[channel.Name] = group
		}
		return assignment, true, nil
	})

	s := New(Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	s.JoinChannel("#b")
	s.JoinChannel("#c")
	s.PartChannel("#b")

	a, _ := s.Channel("#a")
	c, _ := s.Channel("#c")
	if a.Group != 7 || c.Group != 7 {
		t.Fatalf("remaining groups=%d,%d want 7,7", a.Group, c.Group)
	}
	if got := s.NumericGroupCount(7); got != 2 {
		t.Fatalf("group 7 count=%d want 2", got)
	}
	if got := s.NormalChannelCount(); got != 2 {
		t.Fatalf("normal count=%d want 2", got)
	}
}

func TestGroupingValidationFailureFallsBackToSeedPlacement(t *testing.T) {
	strategy := groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
		assignment := make(Assignment, 0)
		for _, channel := range input.Channels {
			if channel.Name == input.ChangedChannel {
				continue
			}
			assignment[channel.Name] = 0
		}
		return assignment, true, nil
	})

	s := New(Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	s.JoinChannel("#b")

	a, _ := s.Channel("#a")
	b, _ := s.Channel("#b")
	if a.Group != 0 || b.Group != 1 {
		t.Fatalf("fallback placement mismatch, groups=%d,%d want 0,1", a.Group, b.Group)
	}

	logged := false
	for _, event := range s.Events() {
		if strings.Contains(event, "grouping assignment rejected") {
			logged = true
			break
		}
	}
	if !logged {
		t.Fatal("expected grouping assignment rejection event")
	}
}

func TestGroupingStrategyErrorFallsBackToSeedPlacement(t *testing.T) {
	strategy := groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
		return nil, false, errors.New("broken strategy")
	})

	s := New(Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")

	a, _ := s.Channel("#a")
	if a.Group != 0 {
		t.Fatalf("fallback placement mismatch, group=%d want 0", a.Group)
	}

	logged := false
	for _, event := range s.Events() {
		if strings.Contains(event, "grouping strategy error") {
			logged = true
			break
		}
	}
	if !logged {
		t.Fatal("expected grouping strategy error event")
	}
}
