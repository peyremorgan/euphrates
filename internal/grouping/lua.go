package grouping

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"euphrates/internal/state"

	lua "github.com/yuin/gopher-lua"
)

const (
	defaultLuaTimeout  = 100 * time.Millisecond
	defaultLuaMemoryMx = 8 * 1024 * 1024
)

// LoadDir loads *.lua files from dir in lexical filename order and returns a
// chain ending with the built-in least-populated fallback strategy.
func LoadDir(dir string) (state.GroupingStrategy, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".lua") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	strategies := make([]state.GroupingStrategy, 0, len(names)+1)
	for _, name := range names {
		strategy, err := newLuaStrategy(filepath.Join(dir, name), defaultLuaTimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		strategies = append(strategies, strategy)
	}
	strategies = append(strategies, state.LeastPopulatedStrategy{})
	return state.GroupingChain(strategies), nil
}

type luaStrategy struct {
	name    string
	timeout time.Duration

	mu sync.Mutex
	L  *lua.LState
	fn *lua.LFunction
}

func newLuaStrategy(path string, timeout time.Duration) (*luaStrategy, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	if err := openLuaLib(L, lua.BaseLibName, lua.OpenBase); err != nil {
		L.Close()
		return nil, err
	}
	if err := openLuaLib(L, lua.TabLibName, lua.OpenTable); err != nil {
		L.Close()
		return nil, err
	}
	if err := openLuaLib(L, lua.StringLibName, lua.OpenString); err != nil {
		L.Close()
		return nil, err
	}
	if err := openLuaLib(L, lua.MathLibName, lua.OpenMath); err != nil {
		L.Close()
		return nil, err
	}
	L.SetMx(defaultLuaMemoryMx)

	chunk, err := L.LoadFile(path)
	if err != nil {
		L.Close()
		return nil, err
	}
	L.Push(chunk)
	if err := L.PCall(0, 1, nil); err != nil {
		L.Close()
		return nil, err
	}
	ret := L.Get(-1)
	L.Pop(1)
	fn, ok := ret.(*lua.LFunction)
	if !ok {
		L.Close()
		return nil, fmt.Errorf("script must return a function")
	}

	return &luaStrategy{name: filepath.Base(path), timeout: timeout, L: L, fn: fn}, nil
}

func openLuaLib(L *lua.LState, name string, lib lua.LGFunction) error {
	return L.CallByParam(lua.P{Fn: L.NewFunction(lib), NRet: 0, Protect: true}, lua.LString(name))
}

func (s *luaStrategy) Apply(input state.GroupingInput) (state.Assignment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	s.L.SetContext(ctx)
	defer s.L.RemoveContext()

	ctxTable := s.buildContextTable(input)
	if err := s.L.CallByParam(lua.P{Fn: s.fn, NRet: 1, Protect: true}, ctxTable); err != nil {
		return nil, false, fmt.Errorf("%s: %w", s.name, err)
	}
	ret := s.L.Get(-1)
	s.L.Pop(1)

	if ret == lua.LNil || ret == lua.LFalse {
		return nil, false, nil
	}
	table, ok := ret.(*lua.LTable)
	if !ok {
		return nil, false, fmt.Errorf("%s: strategy return must be table, nil, or false", s.name)
	}

	assignment := make(state.Assignment, table.Len())
	parseErr := error(nil)
	table.ForEach(func(key lua.LValue, value lua.LValue) {
		if parseErr != nil {
			return
		}
		name, ok := key.(lua.LString)
		if !ok {
			parseErr = fmt.Errorf("%s: assignment key must be string", s.name)
			return
		}
		number, ok := value.(lua.LNumber)
		if !ok {
			parseErr = fmt.Errorf("%s: assignment value for %q must be number", s.name, string(name))
			return
		}
		group := float64(number)
		if math.Trunc(group) != group {
			parseErr = fmt.Errorf("%s: assignment value for %q must be integer", s.name, string(name))
			return
		}
		assignment[string(name)] = state.GroupID(int(group))
	})
	if parseErr != nil {
		return nil, false, parseErr
	}
	return assignment, true, nil
}

func (s *luaStrategy) buildContextTable(input state.GroupingInput) *lua.LTable {
	ctx := s.L.NewTable()
	s.L.SetField(ctx, "trigger", lua.LString(input.Trigger))
	s.L.SetField(ctx, "changed_channel", lua.LString(input.ChangedChannel))

	channels := s.L.NewTable()
	for _, channel := range input.Channels {
		entry := s.L.NewTable()
		s.L.SetField(entry, "name", lua.LString(channel.Name))
		s.L.SetField(entry, "group", lua.LNumber(channel.Group))
		s.L.SetField(entry, "join_order", lua.LNumber(channel.JoinOrder))
		s.L.SetField(channels, channel.Name, entry)
	}
	s.L.SetField(ctx, "channels", channels)

	groups := s.L.NewTable()
	for i := 0; i < state.NumGroups; i++ {
		group := s.L.NewTable()
		for _, name := range input.Groups[i] {
			group.Append(lua.LString(name))
		}
		groups.RawSetInt(i+1, group)
	}
	s.L.SetField(ctx, "groups", groups)

	return ctx
}
