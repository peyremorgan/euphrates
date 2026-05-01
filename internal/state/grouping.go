package state

import (
	"fmt"
)

// GroupingTrigger identifies which lifecycle event initiated regrouping.
type GroupingTrigger string

const (
	GroupingTriggerJoin GroupingTrigger = "join"
	GroupingTriggerPart GroupingTrigger = "part"
)

// ChannelSnapshot is the script/strategy-facing view of a normal channel.
type ChannelSnapshot struct {
	Name      string
	Group     GroupID
	JoinOrder int
}

// GroupingInput is the immutable input passed to assignment strategies.
type GroupingInput struct {
	Trigger        GroupingTrigger
	ChangedChannel string
	Channels       []ChannelSnapshot
	Groups         [NumGroups][]string
}

// Assignment maps canonical channel keys to numeric group ids.
type Assignment map[string]GroupID

// GroupingStrategy computes a full assignment for all normal channels.
//
// Returning handled=false delegates to the next strategy in a chain.
// Returning handled=true commits the assignment after host validation.
type GroupingStrategy interface {
	Apply(input GroupingInput) (assignment Assignment, handled bool, err error)
}

// GroupingChain composes multiple strategies in order.
type GroupingChain []GroupingStrategy

// Apply runs each strategy until one handles the input.
func (c GroupingChain) Apply(input GroupingInput) (Assignment, bool, error) {
	var firstErr error
	for i, strategy := range c {
		if strategy == nil {
			continue
		}
		assignment, handled, err := strategy.Apply(input)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("strategy %d: %w", i, err)
			}
			continue
		}
		if handled {
			return assignment, true, firstErr
		}
	}
	if firstErr != nil {
		return nil, false, firstErr
	}
	return nil, false, nil
}

// LeastPopulatedStrategy reproduces the historical placement behavior.
type LeastPopulatedStrategy struct{}

// Apply keeps existing placements and places the newly-joined channel in the
// least-populated numeric group. PART leaves all remaining channels in place.
func (LeastPopulatedStrategy) Apply(input GroupingInput) (Assignment, bool, error) {
	assignment := make(Assignment, len(input.Channels))
	counts := [NumGroups]int{}

	for _, channel := range input.Channels {
		if !channel.Group.IsNumeric() {
			return nil, false, fmt.Errorf("channel %q has non-numeric group %d", channel.Name, channel.Group)
		}
		if input.Trigger == GroupingTriggerJoin && channel.Name == input.ChangedChannel {
			continue
		}
		assignment[channel.Name] = channel.Group
		counts[channel.Group]++
	}

	if input.Trigger == GroupingTriggerJoin && input.ChangedChannel != "" {
		changedFound := false
		for _, channel := range input.Channels {
			if channel.Name == input.ChangedChannel {
				changedFound = true
				break
			}
		}
		if changedFound {
			assignment[input.ChangedChannel] = pickNumericGroup(counts)
		}
	}

	return assignment, true, nil
}

// DefaultGroupingStrategy returns the built-in chain used when no custom
// strategy is configured.
func DefaultGroupingStrategy() GroupingStrategy {
	return GroupingChain{LeastPopulatedStrategy{}}
}

// ValidateAssignment enforces assignment safety guarantees.
func ValidateAssignment(input GroupingInput, assignment Assignment) error {
	if len(input.Channels) == 0 {
		if len(assignment) != 0 {
			return fmt.Errorf("assignment has %d entries, want 0", len(assignment))
		}
		return nil
	}
	if len(assignment) != len(input.Channels) {
		return fmt.Errorf("assignment has %d entries, want %d", len(assignment), len(input.Channels))
	}

	expected := make(map[string]struct{}, len(input.Channels))
	for _, channel := range input.Channels {
		expected[channel.Name] = struct{}{}
	}

	for key, group := range assignment {
		if _, ok := expected[key]; !ok {
			return fmt.Errorf("assignment contains unknown channel %q", key)
		}
		if !group.IsNumeric() {
			return fmt.Errorf("assignment for %q has non-numeric group %d", key, group)
		}
		delete(expected, key)
	}

	if len(expected) != 0 {
		for key := range expected {
			return fmt.Errorf("assignment missing channel %q", key)
		}
	}
	return nil
}
