# Initial scaffolding

Full skeleton of the `euphrates` terminal IRC client: state model, tview UI, ircevent wrapper, and a wired `main` entrypoint. Five conventional commits; all tests pass with `-race`; lint clean.

## What was done

- Thread-safe state model: 10k message ring, 5-line event ring, 10 numeric groups + server/queries sentinels, deterministic per-channel colours (FNV-1a → 16-colour palette)
- tview layout: status line, unified scroll pane, events block, channel-prefixed composer; Alt+digit group toggles, Ctrl-N/P channel cycling, `/me` and `/quit` commands
- IRC client wrapping `ircevent.Connection`: pure dispatcher for PRIVMSG, ACTION, NOTICE, JOIN, PART, QUIT, NICK, TOPIC, KICK, MODE and server numerics
- `main` entrypoint with flag parsing (server, TLS, nick, SASL, autojoin channels, ring sizes)

## Issues encountered

### `tview.Application.QueueUpdateDraw` deadlocks without a running event loop

`QueueUpdateDraw` sends on an internal channel that only drains when `app.Run()` is active. Any test that called `handleSubmit` → `OnEvent` → `QueueUpdateDraw` hung indefinitely with a 20-second timeout panic.

**Fix:** introduced a private `addEvent(line string)` method on `UI` that calls `state.AddEvent` + `refreshEvents()` directly (synchronous, no channel send). All input-handler code paths (`handleSubmit`, `handleCommand`, `sendTo`) call `addEvent`; the public `OnEvent` method (with `QueueUpdateDraw`) is kept solely for IRC goroutine callsites.

### `tview.Escape` doesn't cover the `@` character

tview's stock `Escape` function uses the regex `\[(?:[a-zA-Z0-9_,;: \-\."#]+|"...")\]` which does **not** include `@`. This meant `[@alice]` (the query-channel prompt prefix) passed through unescaped and was interpreted by tview as a colour tag, breaking the prompt display.

**Fix:** exported `state.Escape` (formerly the unexported `escapeContent`) which uses the permissive regex `\[([^[\]]*)\]` → `[$1[]`. Both `internal/state/format.go` and `internal/ui/format.go` now use `state.Escape` exclusively; `tview.Escape` is not imported anywhere.

### `TestFormatStatus_TargetVisible` test expectation vs. implementation mismatch

The test was written expecting `"▶ [#foo[]"` (target shown bare), but the design later clarified that the status arrow should mirror the prompt bracket style. The `FormatStatus` implementation was updated to wrap the target in brackets: `"▶ [#foo[]"`, which meant the test was correct and the implementation needed the fix, not the test.

### `ircevent` subpackage not in `go.mod`

The module was declared as `github.com/ergochat/irc-go` in `go.mod` but the import paths inside the code are `github.com/ergochat/irc-go/ircevent` and `github.com/ergochat/irc-go/ircmsg`. These are separate Go packages within the same module and need `go mod tidy` to be added explicitly to `go.sum`. Build failed until `go mod tidy` was re-run after adding the `irc` package.

## Lessons learned

- **tview widget mutation is safe without `Run`**, but anything queued via `QueueUpdate*` is not — it blocks on an unbuffered channel. The pattern of splitting sync (on-loop-goroutine) helpers from async (cross-goroutine) wrappers is essential for testability.
- **`tview.Escape` is narrow by design** — it only escapes sequences that look like its own colour-tag syntax. Any sigil (`@`, `+`, `~`) in a nickname that appears inside `[…]` will fool tview unless a permissive escaper is used.
- **`ircevent` rewrites `PRIVMSG` to `CTCP_ACTION`** for `\x01ACTION …\x01` messages before dispatching callbacks. Register on `"CTCP_ACTION"`, not on `"PRIVMSG"`, to get actions.
- **Numeric callback registration in a loop**: closing over a loop variable in Go 1.22+ is safe (each iteration gets its own `cmd` string), but the callback must capture `m.Command` from the `ircmsg.Message` (which is always correct) rather than the closed-over `cmd` variable to be safe against future Go version behaviour changes. The current code captures `m.Command` correctly.
- **`EnsureChannel` is idempotent**: the State design separates "ensure the channel record exists" from "mark it joined". This matters for queries (PMs), which should appear as channels without the user ever sending `JOIN`.

## Notes for the future

### Things still to implement / figure out

- **Reconnect logic.** `ircevent.Connection` has a built-in reconnect mechanism (set `ReconnectFreq`), but the current wiring doesn't enable it. The UI would need to be notified of reconnect cycles so it can surface them in the events pane.
- **353/366 NAMES list suppression.** Server numerics 001–599 are all routed to the server log. `353` (RPL_NAMREPLY) and `366` (RPL_ENDOFNAMES) produce noisy lines on every join. Consider filtering them out or accumulating them silently.
- **WHOIS / WHOWAS.** Not handled at all.
- **Paste protection / multi-line input.** The `tview.InputField` accepts multi-line paste as a single line. Large pastes should be split or warned about.
- **TLS certificate verification.** Currently `tls.Config.ServerName` is set from `hostOnly(cfg.Server)`, so hostname verification works, but there is no way to pin a certificate or skip verification for self-signed servers (useful for testing against a local Ergo). Add a `-tls-skip-verify` flag if needed.
- **IPv6 literal hosts.** `hostOnly` uses `strings.LastIndexByte(addr, ':')`, which correctly finds the port separator for `[::1]:6697`, but relies on the absence of `]` after the final `:`. Works for the common cases; review if exotic address forms are needed.

### Implementation specifics

- `state.Escape` is the canonical bracket-neutraliser for all tview output. Do not call `tview.Escape` directly anywhere in this codebase.
- `addEvent` (lowercase, unexported) is for event-loop-goroutine call sites; `OnEvent` (exported) is for external goroutines. Mixing them up will cause deadlocks or missed redraws.
- `Client.SetHandlers` must be called **before** `Client.Connect()` — once the connection is active, callbacks run concurrently and there is no mutex protecting the handlers field.
- The `state.GroupServer` and `state.GroupQueries` sentinel `GroupID` values are negative (`-1`, `-2`). Code that iterates numeric groups must use `state.NumGroups` as the upper bound and treat these sentinels separately; do not assume `GroupID` values are always non-negative.
- Channel colours are derived from the lowercase canonical name via FNV-1a hash. The colour is stable across restarts for the same channel name, which is the intended behaviour.
