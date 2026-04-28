# Deterministic Nick Colors for Chat Sender Labels

Implemented deterministic, case-insensitive nick coloring for chat sender labels (PRIVMSG/ACTION/NOTICE) using the same shared color-selection logic already used for channel colors.

## Issues encountered

- One UI test failed after the formatter change because local echo output changed from plain `<me>` to a color-tagged nick label. The fix was to update the UI assertion to match the new state-layer formatting contract.
- No lint or runtime issues were encountered after aligning tests with the new rendering behavior.

## Lessons learned

- Message rendering behavior is owned by the state layer, not the UI layer. The decisive implementation points were in `internal/state/color.go` and `internal/state/format.go`.
- The most robust approach was to share one deterministic color helper and expose thin wrappers (`ChannelColor`, `UserColor`) instead of duplicating hash/palette logic.
- Escaping order matters: nick content must be escaped before wrapping in color tags, otherwise bracketed nick content can interfere with tview formatting.
- Existing test coverage in state and UI made regression detection fast; changing formatter output can ripple into UI tests even when UI code itself is untouched.

## Notes for the future

- Current scope is intentionally limited: only sender labels in chat messages are colorized. Synthetic event lines (join/part/quit/topic/kick text) are still plain text.
- Channel and nick colors now depend on the same deterministic selection logic. If the palette or hashing strategy changes later, both behaviors will change together.
- Nick color matching is case-insensitive by design to stay consistent with existing channel canonicalization and common IRC behavior assumptions.
- If future work requires distinct visual systems for channels vs nicks, consider keeping shared canonicalization/hash plumbing while splitting palette selection to avoid logic drift.
- When modifying formatter output, check both state package tests and UI package tests, because UI expectations often assert rendered line substrings produced by state.
