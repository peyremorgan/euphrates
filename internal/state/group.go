package state

// ChanKind classifies channels by what group bucket they belong to.
type ChanKind int

const (
	// ChanNormal is a regular IRC channel (#foo, &bar, +baz, !qux).
	// It is assigned to one of the numeric groups 0..NumGroups-1.
	ChanNormal ChanKind = iota
	// ChanQuery is a private conversation with another user. All queries
	// share the special GroupQueries bucket.
	ChanQuery
	// ChanServer is the synthetic channel that holds server-originated
	// messages. There is exactly one and it lives in GroupServer.
	ChanServer
)

// GroupID identifies one of the visibility groups. Numeric groups are 0..9;
// GroupServer (-1) and GroupQueries (-2) are special, single-purpose buckets.
type GroupID int

const (
	// NumGroups is the count of numeric, user-toggleable groups.
	NumGroups = 10

	// GroupServer holds server messages only.
	GroupServer GroupID = -1
	// GroupQueries holds all private queries.
	GroupQueries GroupID = -2
)

// IsNumeric reports whether g is a numeric (1-0 hotkey) group.
func (g GroupID) IsNumeric() bool { return g >= 0 && g < NumGroups }

// classifyName returns the ChanKind implied by the textual form of a name.
// Empty name is treated as ChanNormal (callers should avoid that).
func classifyName(name string) ChanKind {
	if name == ServerChannelName {
		return ChanServer
	}
	if name == "" {
		return ChanNormal
	}
	switch name[0] {
	case '#', '&', '+', '!':
		return ChanNormal
	default:
		return ChanQuery
	}
}

// Channel is the metadata stored for each tracked target: a regular channel,
// a query, or the synthetic server channel.
type Channel struct {
	Name      string
	Kind      ChanKind
	Group     GroupID
	JoinOrder int
}

// pickNumericGroup picks the least-populated numeric group, breaking ties by
// lowest index. counts[i] is the population of group i.
func pickNumericGroup(counts [NumGroups]int) GroupID {
	bestIdx := 0
	bestCount := counts[0]
	for i := 1; i < NumGroups; i++ {
		if counts[i] < bestCount {
			bestIdx = i
			bestCount = counts[i]
		}
	}
	return GroupID(bestIdx)
}
