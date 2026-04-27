# euphrates

A terminal IRC client with a unified, group-toggleable chat view.

## Concept

Instead of channel tabs, every joined channel is assigned to one of 10 numeric
**groups** (plus two special groups: server messages and private queries).
Channel messages are interleaved in a single scroll pane prefixed with
`[#channel]`. **Alt+1**…**Alt+0** toggles the visibility of groups 1-10
(think `nmon` modules). **F1**…**F10** solos a numeric group (shows
only that one numeric group). Solo does not change visibility of the
special server/query groups. Bare digits stay reserved for text input in
the composer.

The composer is prefixed with the current target channel in brackets; the
prefix dims when the target's group is hidden, and submitting a message
force-shows the target group again. Cycle through joined channels with
**Ctrl-N** / **Ctrl-P**.

## Layout

```
groups: 1 2 3 4 5 6 7 8 9 0  S Q   ▶ [#foo]   ← status line
[#foo] <alice> hello                          ← unified scroll pane
[#bar] <bob>   hi
…
→ alice joined #foo                           ← 5-line events pane
± #bar mode +o alice
[#foo]                                        ← prompt + input
```

## Build

```
make
```

## Run

```
./euphrates -server irc.libera.chat:6697 -tls -nick yournick -channels '#foo,#bar'
```

Run `./euphrates -h` for the full flag list (SASL, server password, ring
capacities).

## Keys

| Key            | Action                                  |
| -------------- | --------------------------------------- |
| Alt+1..0       | Toggle visibility of numeric group 1-10 |
| F1..F10        | Solo numeric group 1-10                 |
| Ctrl-N         | Next channel (target)                   |
| Ctrl-P         | Previous channel (target)               |
| Up / Down      | Scroll main message history (input empty) |
| Ctrl-C         | Quit                                    |
| `/me <text>`   | Send a CTCP ACTION to the target        |
| `/quit [text]` | Disconnect                              |

## Testing

```
make test       # plain
make test-race  # with the race detector
make lint       # vet + gofmt + (optional) golangci-lint
```
