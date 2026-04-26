package state

import "testing"

func TestClassifyName(t *testing.T) {
	cases := []struct {
		name string
		want ChanKind
	}{
		{"#foo", ChanNormal},
		{"&local", ChanNormal},
		{"+modeless", ChanNormal},
		{"!safe", ChanNormal},
		{"alice", ChanQuery},
		{ServerChannelName, ChanServer},
		{"", ChanNormal},
	}
	for _, c := range cases {
		if got := classifyName(c.name); got != c.want {
			t.Errorf("classifyName(%q)=%v want %v", c.name, got, c.want)
		}
	}
}

func TestGroupID_IsNumeric(t *testing.T) {
	if !GroupID(0).IsNumeric() || !GroupID(NumGroups-1).IsNumeric() {
		t.Error("expected numeric range to be IsNumeric")
	}
	if GroupID(NumGroups).IsNumeric() {
		t.Error("out-of-range marked numeric")
	}
	if GroupServer.IsNumeric() || GroupQueries.IsNumeric() {
		t.Error("special groups marked numeric")
	}
}

func TestPickNumericGroup_PicksLeastPopulated(t *testing.T) {
	var c [NumGroups]int
	for i := range c {
		c[i] = 5
	}
	c[3] = 1
	if got := pickNumericGroup(c); got != GroupID(3) {
		t.Errorf("got %d want 3", got)
	}
}

func TestPickNumericGroup_TieBreaksLowestIndex(t *testing.T) {
	var c [NumGroups]int
	if got := pickNumericGroup(c); got != GroupID(0) {
		t.Errorf("all-zero -> %d, want 0", got)
	}
	for i := range c {
		c[i] = 1
	}
	c[2] = 0
	c[7] = 0
	if got := pickNumericGroup(c); got != GroupID(2) {
		t.Errorf("tie -> %d, want 2", got)
	}
}
