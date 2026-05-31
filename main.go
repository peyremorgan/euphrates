// Command euphrates is a terminal IRC client built on github.com/rivo/tview
// and github.com/ergochat/irc-go. See README.md for the keybindings.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"euphrates/internal/grouping"
	"euphrates/internal/irc"
	"euphrates/internal/state"
	"euphrates/internal/ui"
)

const defaultGroupingScriptDir = "examples/grouping"

// shippedGroupingScripts contains starter Lua strategies copied into the
// user's grouping directory on first run.
//
//go:embed examples/grouping/*.lua
var shippedGroupingScripts embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "euphrates: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		server   = flag.String("server", "", "IRC server host[:port] (required)")
		nick     = flag.String("nick", "", "nickname (required)")
		user     = flag.String("user", "", "username/ident (defaults to nick)")
		real     = flag.String("realname", "", "real name / gecos (defaults to nick)")
		password = flag.String("password", "", "server password (PASS)")
		saslUser = flag.String("sasl-user", "", "SASL PLAIN login")
		saslPass = flag.String("sasl-pass", "", "SASL PLAIN password")
		useTLS   = flag.Bool("tls", false, "wrap the connection in TLS")
		channels = flag.String("channels", "", "comma-separated list of channels to autojoin")
		msgCap   = flag.Int("scrollback", 10000, "main pane scrollback ring capacity")
		evtCap   = flag.Int("events", 200, "events pane ring capacity")
		groupDir = flag.String("grouping-dir", "", "directory containing ordered *.lua grouping strategies")
	)
	flag.Parse()

	if *server == "" || *nick == "" {
		flag.Usage()
		return fmt.Errorf("--server and --nick are required")
	}

	strategy := state.DefaultGroupingStrategy()
	groupingEvents := make([]string, 0, 2)
	dir, explicit, err := resolveGroupingDir(*groupDir)
	if err != nil {
		return err
	}
	if !explicit {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			written, installErr := installDefaultGroupingScripts(shippedGroupingScripts, defaultGroupingScriptDir, dir)
			if installErr != nil {
				groupingEvents = append(groupingEvents, fmt.Sprintf("grouping defaults install failed: %v", installErr))
			} else if written > 0 {
				groupingEvents = append(groupingEvents, fmt.Sprintf("grouping defaults installed in %s", dir))
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			if explicit {
				return fmt.Errorf("grouping path is not a directory: %s", dir)
			}
			groupingEvents = append(groupingEvents, fmt.Sprintf("grouping disabled: %s is not a directory", dir))
		} else {
			loaded, loadErr := grouping.LoadDir(dir)
			if loadErr != nil {
				groupingEvents = append(groupingEvents, fmt.Sprintf("grouping load failed from %s: %v", dir, loadErr))
			} else {
				strategy = loaded
				groupingEvents = append(groupingEvents, fmt.Sprintf("grouping strategies loaded from %s", dir))
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if explicit {
			return fmt.Errorf("grouping directory not found: %s", dir)
		}
	} else {
		return err
	}

	st := state.New(state.Config{
		MessageCap: *msgCap,
		EventCap:   *evtCap,
		ServerName: irc.HostOnly(*server),
		Grouping:   strategy,
	})
	for _, event := range groupingEvents {
		st.AddEvent(event)
	}

	cli, err := irc.New(irc.Config{
		Server:   *server,
		UseTLS:   *useTLS,
		Nick:     *nick,
		User:     *user,
		RealName: *real,
		Password: *password,
		SASLUser: *saslUser,
		SASLPass: *saslPass,
		Channels: splitChannels(*channels),
	}, irc.Handlers{})
	if err != nil {
		return err
	}

	u := ui.New(st, cli)
	// Re-bind handlers now that the UI exists. Self stays pinned to the
	// live connection nick by irc.New.
	cli.SetHandlers(irc.Handlers{
		Self:          cli.Nick,
		OnMessage:     u.OnMessage,
		OnEvent:       u.OnEvent,
		OnJoin:        u.OnJoin,
		OnPart:        u.OnPart,
		OnUserJoin:    u.OnUserJoin,
		OnUserPart:    u.OnUserPart,
		OnUserQuit:    u.OnUserQuit,
		OnUserNick:    u.OnUserNick,
		OnNames:       u.OnNames,
		OnChannelList: st.SetChannelListCache,
	})

	go func() {
		if err := cli.Connect(); err != nil {
			u.OnEvent("connect failed: " + err.Error())
			return
		}
		cli.Loop()
		u.OnEvent("disconnected")
	}()

	if err := u.Run(); err != nil {
		log.Printf("ui: %v", err)
		return err
	}
	return nil
}

// splitChannels parses the comma-separated --channels flag into a clean slice
// (trimmed, empty entries dropped).
func splitChannels(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveGroupingDir(override string) (path string, explicit bool, err error) {
	if override != "" {
		return override, true, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(homeDir, ".euphrates", "grouping.d"), false, nil
}

func installDefaultGroupingScripts(source fs.FS, sourceDir, targetDir string) (int, error) {
	entries, err := fs.ReadDir(source, sourceDir)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return 0, err
	}

	written := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".lua") {
			continue
		}
		targetPath := filepath.Join(targetDir, name)
		if _, err := os.Stat(targetPath); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return written, err
		}

		content, err := fs.ReadFile(source, filepath.Join(sourceDir, name))
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return written, err
		}
		written++
	}

	return written, nil
}
