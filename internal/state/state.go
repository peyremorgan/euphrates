package state

import (
	"sort"
	"strings"
)

// Defaults applied when Config fields are zero.
const (
	defaultMessageCap = 10_000
	defaultEventCap   = 5
)

// Config configures a new State.
type Config struct {
	// MessageCap is the capacity of the unified message ring buffer.
	// Zero means defaultMessageCap.
	MessageCap int
	// EventCap is the number of recent events kept for the events block.
	// Zero means defaultEventCap.
	EventCap int
	// ServerName is the short server label shown in the UI status bar.
	ServerName string
	// Grouping selects the channel-to-group assignment strategy chain.
	// Nil means DefaultGroupingStrategy().
	Grouping GroupingStrategy
}

// State is the in-memory model of the client. All exported methods are safe
// for concurrent use.
type State struct {
	mu guard

	messages *Ring[Message]
	events   *Ring[string]

	channels  map[string]*Channel // canonicalKey -> Channel
	order     []string            // canonical names in insertion order
	nextOrder int
	listCache map[string]string // canonicalKey -> original name from LIST

	counts  [NumGroups]int   // per-numeric-group channel count
	visible map[GroupID]bool // group -> visible (default true)

	target string // canonical name of the current send target ("" if none)

	serverName string
	grouping   GroupingStrategy
}

// New constructs a State with defaults applied for unset Config fields.
func New(cfg Config) *State {
	if cfg.MessageCap <= 0 {
		cfg.MessageCap = defaultMessageCap
	}
	if cfg.EventCap <= 0 {
		cfg.EventCap = defaultEventCap
	}
	if cfg.Grouping == nil {
		cfg.Grouping = DefaultGroupingStrategy()
	}
	s := &State{
		messages:   NewRing[Message](cfg.MessageCap),
		events:     NewRing[string](cfg.EventCap),
		channels:   make(map[string]*Channel),
		listCache:  make(map[string]string),
		visible:    make(map[GroupID]bool),
		serverName: cfg.ServerName,
		grouping:   cfg.Grouping,
	}
	for i := 0; i < NumGroups; i++ {
		s.visible[GroupID(i)] = true
	}
	s.visible[GroupServer] = true
	s.visible[GroupQueries] = true
	return s
}

// --- channel management -----------------------------------------------------

// EnsureChannel adds the given channel if missing and returns its record.
// Group placement:
//   - ChanNormal: least-populated numeric group (ties broken by lowest index)
//   - ChanQuery:  GroupQueries
//   - ChanServer: GroupServer
//
// EnsureChannel is idempotent; calling it on an existing name returns the
// existing record without modification.
func (s *State) EnsureChannel(name string) *Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureChannelLocked(name)
}

// JoinChannel is an alias for EnsureChannel that reads as more natural at the
// IRC-event-handler call site.
func (s *State) JoinChannel(name string) *Channel { return s.EnsureChannel(name) }

func (s *State) ensureChannelLocked(name string) *Channel {
	key := canonicalKey(name)
	if c, ok := s.channels[key]; ok {
		return c
	}
	kind := classifyName(name)
	var group GroupID
	switch kind {
	case ChanQuery:
		group = GroupQueries
	case ChanServer:
		group = GroupServer
	default:
		group = pickNumericGroup(s.counts)
		s.counts[group]++
	}
	c := &Channel{
		Name:      name,
		Kind:      kind,
		Group:     group,
		JoinOrder: s.nextOrder,
	}
	s.nextOrder++
	s.channels[key] = c
	s.order = append(s.order, key)
	// First sendable channel auto-becomes the target.
	if s.target == "" && kind != ChanServer {
		s.target = key
	}
	if kind == ChanNormal {
		s.applyGroupingLocked(GroupingTriggerJoin, key)
	}
	return c
}

// PartChannel removes the channel from tracking. Subsequent calls are noops.
// If the parted channel was the current target, the target falls back to the
// next sendable channel in insertion order, or "" if none remain.
func (s *State) PartChannel(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := canonicalKey(name)
	c, ok := s.channels[key]
	if !ok {
		return
	}
	if c.Kind == ChanNormal {
		s.counts[c.Group]--
	}
	delete(s.channels, key)
	for i, k := range s.order {
		if k == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	if s.target == key {
		s.target = s.firstSendableLocked()
	}
	if c.Kind == ChanNormal {
		s.applyGroupingLocked(GroupingTriggerPart, key)
	}
}

// Channel returns a copy of the named channel's record.
func (s *State) Channel(name string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.channels[canonicalKey(name)]; ok {
		return *c, true
	}
	return Channel{}, false
}

// Channels returns a snapshot of all channels in insertion order.
func (s *State) Channels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Channel, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, *s.channels[k])
	}
	return out
}

// SetChannelListCache replaces the local cache of LIST-discovered channels.
// Names are de-duplicated case-insensitively.
func (s *State) SetChannelListCache(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCache = make(map[string]string, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		s.listCache[canonicalKey(name)] = name
	}
}

// MatchJoinableChannels returns cached LIST channels matching prefix,
// excluding channels we already joined. Matching is case-insensitive.
func (s *State) MatchJoinableChannels(prefix string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefixKey := canonicalKey(prefix)
	out := make([]string, 0, len(s.listCache))
	for key, name := range s.listCache {
		if !strings.HasPrefix(key, prefixKey) {
			continue
		}
		if c, ok := s.channels[key]; ok && c.Kind == ChanNormal {
			continue
		}
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool {
		return canonicalKey(out[i]) < canonicalKey(out[j])
	})
	return out
}

// --- messaging --------------------------------------------------------------

// AppendMessage stores m in the message log (auto-creating its channel if
// needed) and returns the formatted line plus whether the line is currently
// visible (i.e. its channel's group is on).
func (s *State) AppendMessage(m Message) (line string, visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.ensureChannelLocked(m.Channel)
	s.messages.Push(m)
	return formatMessage(m, c.Kind), s.visible[c.Group]
}

// AddEvent appends a pre-formatted event line to the events ring.
func (s *State) AddEvent(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.Push(line)
}

// Events returns a snapshot of the recent event lines (oldest first).
func (s *State) Events() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events.Slice()
}

// RenderVisible returns the formatted lines for every currently-visible
// message, oldest first.
func (s *State) RenderVisible() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, s.messages.Len())
	s.messages.ForEach(func(m Message) bool {
		c, ok := s.channels[canonicalKey(m.Channel)]
		if !ok {
			return true
		}
		if !s.visible[c.Group] {
			return true
		}
		out = append(out, formatMessage(m, c.Kind))
		return true
	})
	return out
}

// --- group visibility -------------------------------------------------------

// ToggleGroup flips visibility of g and returns the new state.
func (s *State) ToggleGroup(g GroupID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := !s.visible[g]
	s.visible[g] = v
	return v
}

// SoloNumericGroup makes only the target numeric group visible, hiding the
// other numeric groups. Special groups (server, queries) are left unchanged.
func (s *State) SoloNumericGroup(g GroupID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !g.IsNumeric() {
		return
	}
	for i := 0; i < NumGroups; i++ {
		id := GroupID(i)
		s.visible[id] = id == g
	}
}

// SetVisible sets the visibility of group g.
func (s *State) SetVisible(g GroupID, vis bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible[g] = vis
}

// IsVisible reports the current visibility of g.
func (s *State) IsVisible(g GroupID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visible[g]
}

// IsChannelVisible reports whether messages for the named channel are
// currently displayed. Unknown channels report false.
func (s *State) IsChannelVisible(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[canonicalKey(name)]
	if !ok {
		return false
	}
	return s.visible[c.Group]
}

// NumericGroupCount returns the number of normal channels currently assigned
// to numeric group g. Non-numeric groups always report 0.
func (s *State) NumericGroupCount(g GroupID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !g.IsNumeric() {
		return 0
	}
	return s.counts[g]
}

// NormalChannelCount returns the total number of normal channels currently
// tracked across all numeric groups. Queries and the server pseudo-channel are
// excluded.
func (s *State) NormalChannelCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, count := range s.counts {
		total += count
	}
	return total
}

// ServerName returns the configured server label used in the status bar.
func (s *State) ServerName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serverName
}

// VisibleGroups returns a copy of the current visibility map.
func (s *State) VisibleGroups() map[GroupID]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[GroupID]bool, len(s.visible))
	for k, v := range s.visible {
		out[k] = v
	}
	return out
}

// --- target / cycling -------------------------------------------------------

// Target returns the canonical name of the current send target ("" if none).
func (s *State) Target() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.target
}

// SetTarget sets the target to the named channel if it exists. Server-kind
// channels are not valid targets and are ignored.
func (s *State) SetTarget(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := canonicalKey(name)
	c, ok := s.channels[key]
	if !ok || c.Kind == ChanServer {
		return
	}
	s.target = key
}

// NextChannel cycles the target to the next sendable channel in insertion
// order. Server channels are skipped. Returns the new target name.
func (s *State) NextChannel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cycleLocked(+1)
}

// PrevChannel cycles the target to the previous sendable channel.
func (s *State) PrevChannel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cycleLocked(-1)
}

// ForceTargetVisible ensures the current target's group is visible. Returns
// the affected group (or 0 if nothing was changed / no target).
func (s *State) ForceTargetVisible() GroupID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.target == "" {
		return 0
	}
	c, ok := s.channels[s.target]
	if !ok {
		return 0
	}
	if !s.visible[c.Group] {
		s.visible[c.Group] = true
	}
	return c.Group
}

// PromptLabel returns the tview-tagged label for the composer prompt:
// the bracketed target channel followed by a single space. The label is
// dimmed when the target's group is currently hidden, and empty when there
// is no target.
func (s *State) PromptLabel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.target == "" {
		return ""
	}
	c, ok := s.channels[s.target]
	if !ok {
		return ""
	}
	tag := channelTag(c.Name, c.Kind)
	if !s.visible[c.Group] {
		// Replace the leading color tag with the dim tag so the composer
		// prompt visibly mirrors the group's state.
		// channelTag is "[fg]" + "[name[]" + "[-]"; swap [fg] for dim.
		idxClose := strings.Index(tag, "]")
		if idxClose > 0 {
			tag = dimColor + tag[idxClose+1:]
		}
	}
	return tag + " "
}

// --- internals --------------------------------------------------------------

func (s *State) cycleLocked(dir int) string {
	sendable := s.sendableKeysLocked()
	if len(sendable) == 0 {
		s.target = ""
		return ""
	}
	idx := -1
	for i, k := range sendable {
		if k == s.target {
			idx = i
			break
		}
	}
	if idx < 0 {
		// target absent (or empty/server) -> jump to first/last
		if dir > 0 {
			s.target = sendable[0]
		} else {
			s.target = sendable[len(sendable)-1]
		}
		return s.target
	}
	idx = (idx + dir + len(sendable)) % len(sendable)
	s.target = sendable[idx]
	return s.target
}

// sendableKeysLocked returns the canonical keys of all non-server channels in
// insertion order.
func (s *State) sendableKeysLocked() []string {
	out := make([]string, 0, len(s.order))
	for _, k := range s.order {
		if c := s.channels[k]; c != nil && c.Kind != ChanServer {
			out = append(out, k)
		}
	}
	return out
}

// firstSendableLocked returns the first non-server channel's canonical key,
// or "" if there is none.
func (s *State) firstSendableLocked() string {
	for _, k := range s.order {
		if c := s.channels[k]; c != nil && c.Kind != ChanServer {
			return k
		}
	}
	return ""
}

func (s *State) applyGroupingLocked(trigger GroupingTrigger, changedChannel string) {
	if s.grouping == nil {
		return
	}
	input := s.makeGroupingInputLocked(trigger, changedChannel)
	assignment, handled, err := s.grouping.Apply(input)
	if err != nil {
		s.events.Push("grouping strategy error: " + err.Error())
	}
	if !handled {
		return
	}
	if err := ValidateAssignment(input, assignment); err != nil {
		s.events.Push("grouping assignment rejected: " + err.Error())
		return
	}
	s.applyAssignmentLocked(assignment)
}

func (s *State) makeGroupingInputLocked(trigger GroupingTrigger, changedChannel string) GroupingInput {
	input := GroupingInput{
		Trigger:        trigger,
		ChangedChannel: changedChannel,
	}
	input.Channels = make([]ChannelSnapshot, 0, len(s.channels))

	for _, key := range s.order {
		channel := s.channels[key]
		if channel == nil || channel.Kind != ChanNormal {
			continue
		}
		input.Channels = append(input.Channels, ChannelSnapshot{
			Name:      key,
			Group:     channel.Group,
			JoinOrder: channel.JoinOrder,
		})
		if channel.Group.IsNumeric() {
			input.Groups[channel.Group] = append(input.Groups[channel.Group], key)
		}
	}

	return input
}

func (s *State) applyAssignmentLocked(assignment Assignment) {
	s.counts = [NumGroups]int{}
	for key, group := range assignment {
		channel := s.channels[key]
		if channel == nil || channel.Kind != ChanNormal {
			continue
		}
		channel.Group = group
		s.counts[group]++
	}
}
