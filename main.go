// Command euphrates is a terminal IRC client built on github.com/rivo/tview
// and github.com/ergochat/irc-go. See README.md for the keybindings.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"euphrates/internal/grouping"
	"euphrates/internal/irc"
	"euphrates/internal/state"
	"euphrates/internal/ui"
)

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
	groupingEvent := ""
	dir, explicit, err := resolveGroupingDir(*groupDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			if explicit {
				return fmt.Errorf("grouping path is not a directory: %s", dir)
			}
			groupingEvent = fmt.Sprintf("grouping disabled: %s is not a directory", dir)
		} else {
			loaded, loadErr := grouping.LoadDir(dir)
			if loadErr != nil {
				groupingEvent = fmt.Sprintf("grouping load failed from %s: %v", dir, loadErr)
			} else {
				strategy = loaded
				groupingEvent = fmt.Sprintf("grouping strategies loaded from %s", dir)
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
	if groupingEvent != "" {
		st.AddEvent(groupingEvent)
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(configDir, "euphrates", "grouping.d"), false, nil
}
