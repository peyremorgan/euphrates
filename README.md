# euphrates

A terminal IRC client with a unified, group-toggleable chat view.

## Concept

Instead of channel tabs, every joined channel is assigned to one of 10 numeric
**groups** (plus two special groups: server messages and private queries).
Channel messages are interleaved in a single scroll pane prefixed with
`[#channel]`. Number keys `1`-`0` toggle the visibility of groups 1-10
(think `nmon` modules).

## Build

```
make
```

## Run

```
./euphrates
```
