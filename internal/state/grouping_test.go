package state

import (
	"errors"
	"testing"
)

type groupingStrategyFunc func(GroupingInput) (Assignment, bool, error)

func (f groupingStrategyFunc) Apply(input GroupingInput) (Assignment, bool, error) {
	return f(input)
}

func TestValidateAssignment_RejectsMissingChannel(t *testing.T) {
	input := GroupingInput{
		Channels: []ChannelSnapshot{{Name: "#a", Group: 0}, {Name: "#b", Group: 1}},
	}
	assignment := Assignment{"#a": 0}
	if err := ValidateAssignment(input, assignment); err == nil {
		t.Fatal("expected missing channel validation error")
	}
}

func TestValidateAssignment_RejectsUnknownChannel(t *testing.T) {
	input := GroupingInput{
		Channels: []ChannelSnapshot{{Name: "#a", Group: 0}},
	}
	assignment := Assignment{"#a": 0, "#ghost": 1}
	if err := ValidateAssignment(input, assignment); err == nil {
		t.Fatal("expected unknown channel validation error")
	}
}

func TestValidateAssignment_RejectsNonNumericGroup(t *testing.T) {
	input := GroupingInput{
		Channels: []ChannelSnapshot{{Name: "#a", Group: 0}},
	}
	assignment := Assignment{"#a": GroupQueries}
	if err := ValidateAssignment(input, assignment); err == nil {
		t.Fatal("expected non-numeric group validation error")
	}
}

func TestGroupingChain_DelegatesUntilHandled(t *testing.T) {
	chain := GroupingChain{
		groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
			return nil, false, nil
		}),
		groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
			return Assignment{"#a": 3}, true, nil
		}),
	}
	assignment, handled, err := chain.Apply(GroupingInput{Channels: []ChannelSnapshot{{Name: "#a", Group: 0}}})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if got := assignment["#a"]; got != 3 {
		t.Fatalf("assignment[#a]=%d want 3", got)
	}
}

func TestGroupingChain_PropagatesStrategyError(t *testing.T) {
	boom := errors.New("boom")
	chain := GroupingChain{
		groupingStrategyFunc(func(input GroupingInput) (Assignment, bool, error) {
			return nil, false, boom
		}),
	}
	_, _, err := chain.Apply(GroupingInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLeastPopulatedStrategy_JoinAssignsChangedChannel(t *testing.T) {
	strategy := LeastPopulatedStrategy{}
	input := GroupingInput{
		Trigger:        GroupingTriggerJoin,
		ChangedChannel: "#c",
		Channels: []ChannelSnapshot{
			{Name: "#a", Group: 0},
			{Name: "#b", Group: 1},
			{Name: "#c", Group: 0}, // changed channel should be re-picked
		},
	}
	assignment, handled, err := strategy.Apply(input)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if got := assignment["#c"]; got != 2 {
		t.Fatalf("assignment[#c]=%d want 2", got)
	}
}
