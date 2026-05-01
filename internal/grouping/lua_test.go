package grouping

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"euphrates/internal/state"
)

func writeScript(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func TestLoadDir_OrdersByFilenameAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "90_delegate.lua", `return function(ctx) return nil end`)
	writeScript(t, dir, "10_pin.lua", `
return function(ctx)
  local out = {}
  for name, _ in pairs(ctx.channels) do
    out[name] = 0
  end
  if ctx.channels["#a"] ~= nil then
    out["#a"] = 9
  end
  return out
end
`)

	strategy, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	s := state.New(state.Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	s.JoinChannel("#b")
	a, _ := s.Channel("#a")
	b, _ := s.Channel("#b")
	if a.Group != 9 {
		t.Fatalf("#a group=%d want 9", a.Group)
	}
	if b.Group != 0 {
		t.Fatalf("#b group=%d want 0", b.Group)
	}
}

func TestLoadDir_BadScriptFails(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "10_bad.lua", `return 123`)
	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must return a function") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDir_SandboxBlocksOSLibrary(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "10_os.lua", `
return function(ctx)
  local x = os and os.exit
  if x ~= nil then
    return {}
  end
  return nil
end
`)
	strategy, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	s := state.New(state.Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	a, _ := s.Channel("#a")
	if a.Group != 0 {
		t.Fatalf("#a group=%d want 0", a.Group)
	}
}

func TestLoadDir_TimeoutFallsThroughToDefault(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "10_hang.lua", `
return function(ctx)
  while true do
  end
end
`)
	strategy, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	s := state.New(state.Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	a, _ := s.Channel("#a")
	if a.Group != 0 {
		t.Fatalf("#a group=%d want 0", a.Group)
	}
	foundError := false
	for _, event := range s.Events() {
		if strings.Contains(event, "grouping strategy error") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatal("expected strategy error event for timeout")
	}
}

func TestLoadDir_RejectsAddDeleteMutations(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "10_mutate.lua", `
return function(ctx)
  local out = {}
  out["#ghost"] = 0
  return out
end
`)
	strategy, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	s := state.New(state.Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	s.JoinChannel("#a")
	a, _ := s.Channel("#a")
	if a.Group != 0 {
		t.Fatalf("#a group=%d want 0", a.Group)
	}
	foundReject := false
	for _, event := range s.Events() {
		if strings.Contains(event, "grouping assignment rejected") {
			foundReject = true
			break
		}
	}
	if !foundReject {
		t.Fatal("expected assignment rejection event")
	}
}
