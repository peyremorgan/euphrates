package ui

import (
	"sort"
	"strings"
)

// lcsScore returns the longest common subsequence length between query and
// candidate. It is case-insensitive.
func lcsScore(query, candidate string) int {
	q := []rune(strings.ToLower(query))
	c := []rune(strings.ToLower(candidate))
	if len(q) == 0 || len(c) == 0 {
		return 0
	}

	prev := make([]int, len(c)+1)
	curr := make([]int, len(c)+1)
	for i := 1; i <= len(q); i++ {
		for j := 1; j <= len(c); j++ {
			if q[i-1] == c[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] >= curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for j := 0; j <= len(c); j++ {
			curr[j] = 0
		}
	}
	return prev[len(c)]
}

func sortChannelsByLCS(channels []string, query string) []string {
	out := append([]string(nil), channels...)
	if len(out) == 0 || query == "" {
		return out
	}

	type scored struct {
		name  string
		score int
	}
	scoredChannels := make([]scored, 0, len(out))
	for _, name := range out {
		scoredChannels = append(scoredChannels, scored{name: name, score: lcsScore(query, name)})
	}
	sort.Slice(scoredChannels, func(i, j int) bool {
		if scoredChannels[i].score != scoredChannels[j].score {
			return scoredChannels[i].score > scoredChannels[j].score
		}
		return strings.ToLower(scoredChannels[i].name) < strings.ToLower(scoredChannels[j].name)
	})
	for i := range scoredChannels {
		out[i] = scoredChannels[i].name
	}
	return out
}
