# Up/Down Message Scrollback in Main Chat View

Added Up/Down keyboard support to scroll main chat history when the input field is empty, while preserving normal input behavior when text is being typed.

## What Changed

- Implemented Up/Down handling in global key capture to scroll the main message view when composer text is empty.
- Added a manual-scroll mode in the UI so incoming and locally echoed messages do not force-scroll to the bottom while the user is reading history.
- Restored auto-follow behavior once the user scrolls back to the bottom.
- Updated README keybindings to document the new Up/Down behavior and the input-empty condition.
- Added focused unit tests for:
  - Up consumed when input is empty
  - Up passthrough when input has text
  - Down behavior at bottom and manual-scroll reset
  - Scroll offset preservation during full refresh
  - No forced auto-follow on local echo while manually scrolled

## Issues Encountered

- A bottom-edge unit test initially failed in headless test context because TextView bottom-state assumptions were too optimistic without a real draw loop.
- Resolved by improving bottom detection logic in UI code and stabilizing the test setup to deterministic conditions.

## Lessons Learned

- Keyboard routing is centralized and cleanly extensible via application input capture, which made this feature straightforward to integrate without changing focus behavior.
- The main view refresh path was a critical coupling point: unconditional ScrollToEnd calls can silently break any future scrollback features.
- Headless TUI tests can be sensitive to layout/viewport assumptions; explicit rect sizing and controlled content volume significantly improve reliability.

## Notes for the Future

- Manual-scroll state currently depends on view scroll offsets and wrapped-line calculations; changes to wrapping/layout behavior may affect bottom detection correctness.
- If paging keys (PageUp/PageDown) are added later, they should reuse the same manual-scroll model to avoid conflicting auto-follow behavior.
- If message rendering becomes more complex (regions, variable formatting cost), keep an eye on refresh performance and avoid expensive recomputation on every key event.
- When modifying refresh logic, verify both behaviors explicitly:
  - reading old history must not auto-jump
  - normal at-bottom mode must continue to follow new messages
