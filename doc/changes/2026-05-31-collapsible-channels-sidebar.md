# Collapsible Channels Sidebar

Added a collapsible left sidebar that displays all 10 numeric groups (1-0) with their associated channels, toggled via Alt+G. Channels are shown indented under group headers in insertion order. The sidebar uses a fixed width of 30 columns when visible and collapses to 0 when hidden. The main pane and sidebar are separated by a vertical divider, and the sidebar includes a title bar at the top.

## Issues encountered

- **Layout restructuring required**: The existing layout used a vertical `FlexRow` that directly contained `mainView`. To accommodate the sidebar, a new horizontal `contentRow` (`FlexColumn`) had to be inserted to hold both the sidebar and main pane side-by-side. This meant changing `root.AddItem(mainView, ...)` to `root.AddItem(contentRow, ...)` and nesting the main view inside the content row.

- **tview Flex item ordering matters**: Initially attempted to add layout items out of order, which caused the wrong primitives to receive focus. The sidebar must be added to `contentRow` *before* `mainView` to ensure left-to-right ordering. tview's `AddItem` appends items sequentially.

- **Test assertion update for layout depth**: The existing test `TestBuildLayout_IncludesSeparatorBetweenMainAndEvents` checked `root.GetItem(1)` expecting `mainView`, but after introducing `contentRow`, item 1 became the content row container. The test was extended to verify the new layout hierarchy: `root → contentRow → [sidebarView, mainView]`.

## Lessons learned

- **Flex ResizeItem is safe for dynamic width changes**: The `ResizeItem(primitive, fixedSize, proportion)` method can be called repeatedly to change a flex item's dimensions. Setting `fixedSize` to 0 effectively hides a primitive without removing it from the layout, which is ideal for toggle behavior. No need to `RemoveItem` and `AddItem` on each toggle.

- **TextView scrollability is independent per primitive**: Both `mainView` and `sidebarView` can be scrollable simultaneously. tview handles scroll offsets per-TextView, so user scrolling in one does not affect the other. The sidebar's `SetScrollable(true)` enables keyboard navigation (j/k/arrows) for long channel lists without additional implementation.

- **State.Channels() preserves insertion order**: The `Channels()` method returns channels in their join order (via the internal `order []string` slice), which is exactly what's needed for sidebar rendering. No additional sorting or timestamp tracking was required.

- **GroupID.IsNumeric() simplifies filtering**: The `IsNumeric()` method on `GroupID` cleanly distinguishes numeric groups (0-9) from special groups (`GroupServer`, `GroupQueries`). This made sidebar filtering straightforward: iterate `Channels()`, skip any channel where `!c.Group.IsNumeric()`.

- **digitForGroup() reuse**: The existing `digitForGroup(i int) string` helper in `format.go` maps group index to display digit (1-9, then 0). This was directly reusable for sidebar headers without reimplementing the mapping logic.

- **unicode.ToLower for case-insensitive key matching**: Alt+g and Alt+G should both toggle the sidebar. Using `unicode.ToLower(ev.Rune()) == 'g'` handles both cases in a single check, avoiding duplicate keybinding logic.

- **RefreshAll is called once at startup**: Adding `u.refreshSidebar()` to `RefreshAll()` ensures the sidebar is populated on initial render. Without this, the sidebar would be blank until the first channel join or group toggle.

- **OnJoin is async-safe**: The `OnJoin` hook uses `app.QueueUpdateDraw()` to marshal sidebar refresh to the tview event loop. This is thread-safe and matches the existing pattern for UI updates from the IRC client goroutine.

## Notes for the future

- **Sidebar width is a compile-time constant**: `sidebarWidth = 30` is hardcoded. Making this user-configurable (e.g., via command-line flag or config file) would improve flexibility for different terminal sizes. The constant is defined in one place and used in `toggleSidebar()`, so changing it is straightforward.

- **Empty groups always show headers**: The sidebar displays all 10 group headers even when a group has no channels. This was a deliberate design choice for consistency with the status bar (which always shows all 10 digit indicators). An alternative would be to hide headers for empty groups, but that would make the sidebar height dynamic and potentially confusing when toggling group visibility.

- **Channel sorting within groups is insertion-order only**: Channels appear in the order they were joined. Alphabetical sorting or custom ordering (e.g., by activity, user count) would require additional logic in `refreshSidebar()`. The current implementation relies on `state.Channels()` order, which is stable and predictable.

- **No visual indication of group visibility state**: The sidebar currently shows all channels regardless of whether their group is visible in the main pane. Adding dimming or color-coding for hidden groups (similar to the status bar's bright/dim styling) would provide better visual feedback. The state method `IsVisible(g GroupID)` is available for this.

- **Sidebar does not receive focus**: The sidebar is marked with `focus: false` in `contentRow.AddItem()`. Keyboard input always goes to the main input field. If sidebar navigation keybindings are added in the future (e.g., j/k to scroll, Enter to set target), focus management will need to be implemented.

- **refreshSidebar is called on every structural change**: Any operation that triggers `refreshAfterStructuralChange()` (toggle, solo, part) rebuilds the entire sidebar text. For large channel lists (hundreds of channels), this involves string concatenation and group filtering. Performance is fine in practice, but if this becomes a bottleneck, the sidebar could track deltas (add/remove single channel) instead of full rebuilds.

- **Server and Query groups excluded from sidebar**: The sidebar only shows numeric groups 1-0. Server messages (`GroupServer`) and private queries (`GroupQueries`) are accessible via the status bar and main pane but not displayed in the sidebar. This was intentional to keep the sidebar focused on regular channels, but it could be revisited if users need quick access to query/server channel lists.

- **Sidebar rendering uses state.Escape()**: Channel names are passed through `state.Escape()` to prevent tview tag injection. This is consistent with other UI components and ensures channels with bracket characters (unlikely but possible in some IRC networks) don't break the sidebar layout.

- **toggleSidebar does not persist state**: Sidebar visibility resets to hidden on every client restart. Persisting `sidebarVisible` to a config file or using an environment variable would allow users to set a default state.

- **Divider and title bar additions**: After the initial sidebar implementation, a 1-column vertical divider was added between the sidebar and main pane for visual separation, and a 1-line title bar ("Channels") was added at the top of the sidebar. These are cosmetic improvements but affect the layout hierarchy. The divider is a separate `TextView` in `contentRow`, and the title bar is a `TextView` in a nested `FlexRow` within the sidebar column.

- **Alt+G conflicts**: Currently no known conflicts with Alt+G in common terminals, but some terminal emulators may intercept Alt key combinations for their own shortcuts. Users in such environments may need to remap the keybinding. The keybinding is centralized in `handleKey()` for easy modification.

- **Sidebar scrolling keybindings**: tview's default TextView navigation (j/k/g/G/arrows) works in the sidebar when it has focus, but since the sidebar never receives focus in the current implementation, these keys are unused. Adding explicit sidebar scroll keybindings (e.g., Ctrl+[/Ctrl+]) would require focus management or a custom input handler that routes specific keys to `sidebarView.ScrollTo()`.
