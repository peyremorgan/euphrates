# Collapsible Users Panel with Live Activity Tracking

Added a collapsible right-side users panel that displays all users across joined normal channels, toggled via Alt+U. The panel shows a deduplicated and activity-sorted list of users, with the most recently active users at the top. Users in the currently-selected target channel are marked with a bullet indicator (`•`). The panel header displays the total user count using decimal SI formatting (e.g., 42, 1.2k, 53M). Like the channels sidebar, the users panel has a fixed width of 30 columns when visible and collapses to 0 when hidden, with a vertical divider separating it from the main pane.

## User list sorting and display behavior

- **Activity-based sorting:** Users who have sent messages are sorted by their most recent activity timestamp (newest first). Users with no tracked activity appear below active users, sorted alphabetically by canonical nickname.

- **Deduplication across channels:** Each user appears exactly once in the global list, even if they are present in multiple joined channels. The state layer maintains per-channel membership sets and merges them for display.

- **Active-channel membership indicator:** Users present in the currently-targeted channel are prefixed with a bullet (`• `) marker. Users in other joined channels have no prefix (two spaces of padding). This helps quickly identify who is in the active conversation context.

- **Nick coloring preserved:** User nicknames are rendered with the same deterministic color scheme used in the main message pane, ensuring visual consistency between the users panel and chat messages.

- **Decimal SI count formatting:** The panel header shows "Users — N" where N is formatted with SI suffixes (k, M, G, etc.) for counts ≥1000. Values under 10 show one decimal place (e.g., 2.7k), and values ≥10 are rounded to integers (e.g., 53k). This keeps the header compact even with large user counts.

## State-layer membership tracking

The state layer now maintains comprehensive user membership and activity records:

- **Per-channel membership sets:** A `map[channelKey]map[userKey]struct{}` tracks which users are present in each normal channel. Keys are canonicalized using the existing `canonicalKey()` function for case-insensitive IRC semantics.

- **Display nick mapping:** A `map[userKey]string` stores the original display case for each user's nickname (e.g., "Alice" vs "alice"), ensuring the users panel shows the most recently observed casing.

- **Activity timestamps:** A `map[userKey]time.Time` records the most recent message timestamp for each user. This powers the activity-based sort and is updated automatically when `AppendMessage()` is called with a non-empty nick.

- **Pruning on disconnect:** When a user parts a channel, quits the network, or a channel is parted, the state layer prunes user records that are no longer present in any tracked channel. This prevents unbounded memory growth and keeps the users list relevant.

- **Rename propagation:** NICK changes update all membership maps and the activity timestamp map, transferring the old user's activity timestamp to the new nick if it's more recent than any existing timestamp for the new nick. This handles nick-change collisions gracefully.

## IRC event wiring

The IRC dispatcher now exposes callbacks for fine-grained membership tracking:

- **OnUserJoin(channel, nick):** Fired when any user (including self) joins a channel. Adds the user to the channel's membership set.

- **OnUserPart(channel, nick):** Fired when any user parts a single channel. Removes the user from that channel's set and prunes if no longer in any channel.

- **OnUserQuit(nick):** Fired when any user quits the network. Removes the user from all channel sets and prunes immediately.

- **OnUserNick(oldNick, newNick):** Fired when any user changes their nickname. Updates membership sets, display name, and activity timestamp across all channels.

- **OnNames(channel, nicks):** Fired when receiving RPL_NAMREPLY (353) from the server. Replaces the entire user set for a channel, pruning users who were previously tracked but are no longer in the names list. This provides an authoritative snapshot on join.

The dispatcher now handles RPL_NAMREPLY (353) parsing, stripping channel membership prefixes (`@`, `+`, `~`, `&`, `%`) and extracting clean nicknames. The test IRC server helper was enhanced to emit realistic NAMES replies on JOIN for end-to-end coverage.

## Issues encountered

- **Initial e2e test deadlock:** The first version of the users panel e2e test called `OnJoin()` and `OnNames()` directly from the test goroutine, which triggered `app.QueueUpdateDraw()` calls that blocked indefinitely because the tview application loop was not running. This was fixed by calling state methods directly (`state.JoinChannel()`, `state.SetChannelUsers()`) and manually invoking `refreshUsersPanel()` instead of relying on async UI updates.

- **Layout item count mismatch in existing tests:** Adding the users panel increased `contentRow.GetItemCount()` from 3 to 5 (sidebar column, sidebar divider, main view, **users divider, users column**). The layout structure test needed updating to verify all 5 items and their types.

- **Divider refresh timing:** The users divider requires the same height-based refresh logic as the sidebar divider. Initially forgot to call `refreshUsersDivider()` in the `SetBeforeDrawFunc()` hook, which caused the divider to render at height 0. Added it alongside `refreshSidebarDivider()`.

- **NAMES reply format variations:** RPL_NAMREPLY can include extended formats like userhost-in-names (nick!user@host). The dispatcher's `stripNamesPrefix()` function needed to handle both plain nicks and full n!u@h formats, truncating at the first `!` character after prefix stripping.

## Lessons learned

- **Membership tracking granularity matters for QUIT vs PART:** PART removes a user from one channel, while QUIT removes them from all channels. The state API reflects this distinction with separate `RemoveUserFromChannel(channel, nick)` and `RemoveUserFromAllChannels(nick)` methods. The dispatcher routes IRC PART and QUIT events to the appropriate method.

- **Activity timestamp must update on every message:** The original implementation only updated activity in `AppendMessage()`. This was sufficient because all normal channel messages flow through that path. Direct-message queries also update activity, which is correct—private message activity should be tracked.

- **Pruning logic prevents memory leaks:** Without pruning, the user maps would grow unbounded as users join and leave over time. The `pruneUserLocked()` helper checks if a user exists in any channel's membership set before deleting their display name and activity timestamp. This is called after every removal operation (part, quit, set-users).

- **SetChannelUsers() replaces, not merges:** When the server sends a NAMES reply, it's authoritative—any user previously in the channel who isn't in the new list has left. The implementation computes the diff (prev - next) and prunes those users. This handles edge cases like netsplits where many users disappear simultaneously.

- **UsersSortedByActivity() deduplicates via seen map:** The sorting function iterates all channel membership sets and collects unique user keys into a `map[userKey]struct{}` before building the sorted slice. This ensures each user appears exactly once even if they're in 10+ channels.

- **Sort stability for inactive users:** `sort.Slice()` is stable, so inactive users (no timestamp) retain their relative order from the `seen` map iteration, which is deterministic within a single call but may vary between calls due to Go map iteration randomness. The alphabetic fallback ensures a consistent user experience.

- **Canonical key usage is consistent:** Both channel and user keys use `canonicalKey()`, which lowercases strings for IRC's case-insensitive semantics. This prevents duplication from case variations (Alice/ALICE/alice all map to the same user).

- **tview resizing pattern is identical to sidebar:** The users panel toggle uses the same `ResizeItem(primitive, fixedSize, proportion)` pattern as the sidebar. Setting fixed size to 0 hides the panel; setting it to `usersPanelWidth` shows it. The divider is toggled similarly (0 vs 1 column).

- **Test IRC server behavior affects e2e coverage:** The in-process IRC server helper (`e2e_helpers_test.go`) needed to emit realistic NAMES numerics (353/366) to exercise the full membership flow. Adding `channelNicks()` and server-side JOIN hooks provides this without needing an external IRC server for unit tests.

## Notes for the future

- **Users panel width is a compile-time constant:** Like the sidebar, `usersPanelWidth = 30` is hardcoded in `ui.go`. Making this configurable would allow users with wide terminals to allocate more space to the users list, or narrow terminals to use less. The constant is used only in `toggleUsersPanel()`, so changing it is trivial.

- **No visual grouping by channel:** The users panel shows a flat, deduplicated list. An alternative design could group users by channel (with collapsible channel headers), but this would make the panel much taller and complicate the active-channel indicator logic. The current design prioritizes compactness and quick scanning.

- **Activity timestamps are message-based only:** User activity is only updated when they send PRIVMSG or ACTION. Other IRC events (MODE changes, TOPIC sets, KICK) do not update activity. This is intentional—those events are visible in the main pane but shouldn't reorder the users list. If richer activity tracking is desired, the `UpdateUserActivity()` API could be called from additional IRC event handlers.

- **No persistence of activity timestamps:** Activity tracking resets on client restart. All users start with zero timestamps. This is acceptable for a lightweight client, but persisting timestamps to disk (e.g., in a JSON cache file) would provide better continuity across sessions.

- **Users panel does not receive focus:** Like the sidebar, the users panel is marked `focus: false`. Keyboard navigation (j/k/arrows) could be added in the future by routing specific keybindings to `usersView.ScrollTo()`, but focus management would need careful handling to avoid breaking the main input field.

- **SI formatting rounds aggressively:** The `formatDecimalSI()` function rounds to one decimal place for values <10, and to integers for values ≥10. This was chosen for compactness, but may lose precision for counts like 9,999 (shown as "10k"). If exact counts are important, the header could display both SI and exact count, or use a tooltip/status line.

- **Indicator prefix width is fixed:** The bullet marker uses a fixed 2-character prefix (`• ` or `  `). This is hardcoded in the formatting loop. If future indicator schemes use longer prefixes, the users panel width may need adjustment to prevent name truncation.

- **Color tags increase string length:** Nicknames are wrapped with tview color tags (`[#RRGGBB]nick[-]`), which increases the byte length of the users panel text. For 1000+ users, this could be several KB of text. tview handles this efficiently, but if memory becomes a concern, colors could be omitted from the users panel (showing plain nicks) or computed lazily during rendering.

- **Users panel refreshes on every message:** `OnMessage()` triggers `refreshUsersPanel()` to update the activity sort and active-channel indicator. For high-traffic channels (100+ messages/second), this could be expensive. A rate-limiting or debouncing mechanism could be added if performance issues arise, though tview's `QueueUpdateDraw()` already coalesces multiple draw requests within a single event-loop tick.

- **Alt+U keybinding conflicts:** Like Alt+G, the Alt+U binding may conflict with terminal emulator shortcuts. The keybinding is centralized in `handleKey()` for easy remapping if needed. No known conflicts in common terminals (iTerm2, gnome-terminal, Windows Terminal).

- **OnUserJoin also fires for self:** The dispatcher calls `OnUserJoin(channel, nick)` for *all* JOIN events, including when the client itself joins. This is correct—the client's own nick should appear in the users panel. The existing `OnJoin(channel)` callback is fired separately for self-joins to trigger sidebar updates and status refreshes.

- **Membership tracking is limited to normal channels:** The state layer only tracks users in `ChanNormal` channels. Server and query channels have `Kind != ChanNormal` and are excluded from membership tracking. This is intentional—query channels are one-to-one conversations (no user list needed), and the server channel is a virtual log (no IRC concept of "users in the server channel").

- **No IRC operator/voice status indicators:** The users panel shows plain nicks without channel operator (`@`) or voice (`+`) prefixes. These prefixes are stripped by `stripNamesPrefix()` during NAMES parsing and are not preserved in the state layer. Adding them would require tracking per-channel per-user privilege mode flags, which is a significant expansion of the membership model.

- **NAMES reply processing is synchronous:** The dispatcher calls `OnNames(channel, nicks)`, which calls `state.SetChannelUsers()`, which iterates and diffs the old/new user sets. For channels with 1000+ users, this is a non-trivial computation on the UI goroutine. If large-channel performance becomes an issue, the diffing logic could be moved to a background goroutine with a completion callback.

- **No support for NAMESX or UHNAMES:** The NAMES parser handles basic RPL_NAMREPLY (353) with standard prefixes. IRCv3 extensions like NAMESX (multiple prefixes) and UHNAMES (full n!u@h in NAMES) are not explicitly handled. The current `stripNamesPrefix()` loop and `!` truncation provide partial compatibility, but full IRCv3 support would require capability negotiation and more complex parsing.

- **Empty users panel behavior:** When no channels are joined, or all joined channels have no users, the users panel body is empty. The header still shows "Users — 0". This is consistent with the empty sidebar behavior (all group headers shown, no channels). An alternative would be to display a placeholder message ("No users tracked"), but the current design keeps it minimal.

- **RefreshUsersPanel is called in OnEvent:** Even non-message events (server notices, join/part notifications) trigger a users panel refresh. This is slightly wasteful (no user activity changed), but ensures the panel stays up-to-date if future enhancements track richer event types. The performance cost is negligible.

- **Test coverage includes dispatcher, state, and UI layers:** The implementation added tests at three levels: (1) dispatcher tests for NAMES parsing and callback invocation, (2) state tests for membership tracking and sorting logic, (3) UI tests for panel rendering and toggle behavior. This ensures all layers are exercised independently and in integration.
