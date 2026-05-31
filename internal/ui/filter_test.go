package ui

import (
	"reflect"
	"testing"
)

func TestLCSScore(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		candidate string
		want      int
	}{
		{name: "exact", query: "abc", candidate: "abc", want: 3},
		{name: "subsequence", query: "aic", candidate: "archiveteam-internal", want: 2},
		{name: "case insensitive", query: "Go", candidate: "#Golang", want: 2},
		{name: "empty query", query: "", candidate: "#go", want: 0},
		{name: "no overlap", query: "xyz", candidate: "#go", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lcsScore(tc.query, tc.candidate); got != tc.want {
				t.Fatalf("lcsScore(%q,%q)=%d want %d", tc.query, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestSortChannelsByLCS(t *testing.T) {
	channels := []string{"#alpha", "#golang", "#games", "#zoo"}
	got := sortChannelsByLCS(channels, "go")
	want := []string{"#golang", "#games", "#zoo", "#alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted=%v want %v", got, want)
	}
	if !reflect.DeepEqual(channels, []string{"#alpha", "#golang", "#games", "#zoo"}) {
		t.Fatalf("input modified: %v", channels)
	}
}

func TestSortChannelsByLCS_EmptyQueryReturnsOriginalOrder(t *testing.T) {
	channels := []string{"#b", "#a"}
	got := sortChannelsByLCS(channels, "")
	want := []string{"#b", "#a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted=%v want %v", got, want)
	}
}
