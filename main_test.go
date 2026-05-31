package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".unused"))

	path, explicit, err := resolveGroupingDir("")
	if err != nil {
		t.Fatalf("resolveGroupingDir: %v", err)
	}
	if explicit {
		t.Fatal("explicit=true want false")
	}
	want := filepath.Join(home, ".euphrates", "grouping.d")
	if path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
}

func TestInstallDefaultGroupingScripts_WritesLuaOnly(t *testing.T) {
	targetDir := t.TempDir()
	source := fstest.MapFS{
		"defaults/010.lua":   {Data: []byte("return function() return nil end")},
		"defaults/020.lua":   {Data: []byte("return function() return nil end")},
		"defaults/README.md": {Data: []byte("ignore")},
	}

	written, err := installDefaultGroupingScripts(source, "defaults", targetDir)
	if err != nil {
		t.Fatalf("installDefaultGroupingScripts: %v", err)
	}
	if written != 2 {
		t.Fatalf("written=%d want 2", written)
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count=%d want 2", len(entries))
	}
}

func TestInstallDefaultGroupingScripts_DoesNotOverwriteExisting(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, "010.lua")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile existing: %v", err)
	}
	source := fstest.MapFS{
		"defaults/010.lua": {Data: []byte("new")},
	}

	written, err := installDefaultGroupingScripts(source, "defaults", targetDir)
	if err != nil {
		t.Fatalf("installDefaultGroupingScripts: %v", err)
	}
	if written != 0 {
		t.Fatalf("written=%d want 0", written)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile existing: %v", err)
	}
	if string(got) != "existing" {
		t.Fatalf("existing content overwritten: %q", got)
	}
}

func TestInstallDefaultGroupingScripts_ShippedScriptsEmbedded(t *testing.T) {
	if _, err := fs.Stat(shippedGroupingScripts, "examples/grouping/010_common_prefix.lua"); err != nil {
		t.Fatalf("missing embedded 010 script: %v", err)
	}
	if _, err := fs.Stat(shippedGroupingScripts, "examples/grouping/020_delimiter_stem.lua"); err != nil {
		t.Fatalf("missing embedded 020 script: %v", err)
	}
}

func TestInstallDefaultGroupingScripts_MissingSourceDirErrors(t *testing.T) {
	_, err := installDefaultGroupingScripts(fstest.MapFS{}, "missing", t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing source dir")
	}
}
