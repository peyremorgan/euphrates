package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitChannels(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "#foo", want: []string{"#foo"}},
		{name: "trim and filter", in: " #foo, ,#bar ,, #baz ", want: []string{"#foo", "#bar", "#baz"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitChannels(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitChannels(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveGroupingDir_Override(t *testing.T) {
	path, explicit, err := resolveGroupingDir("/tmp/custom")
	if err != nil {
		t.Fatalf("resolveGroupingDir: %v", err)
	}
	if !explicit {
		t.Fatal("explicit=false want true")
	}
	if path != "/tmp/custom" {
		t.Fatalf("path=%q want /tmp/custom", path)
	}
}

func TestResolveGroupingDir_Default(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	path, explicit, err := resolveGroupingDir("")
	if err != nil {
		t.Fatalf("resolveGroupingDir: %v", err)
	}
	if explicit {
		t.Fatal("explicit=true want false")
	}
	want := filepath.Join(configDir, "euphrates", "grouping.d")
	if path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
}
