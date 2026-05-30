-- Group a newly joined channel with the existing channel that shares the
-- longest common prefix (minimum 4 characters after channel sigil).
--
-- Input (ctx):
--   ctx.trigger          -> "join" | "part"
--   ctx.changed_channel  -> channel name that triggered regrouping
--   ctx.channels[name]   -> { name = string, group = number, join_order = number }
--
-- Return:
--   nil / false          -> delegate to next strategy
--   table[name] = group  -> full assignment for all channels

local function normalize(name)
  local n = string.lower(name)
  local first = string.sub(n, 1, 1)
  if first == "#" or first == "&" or first == "+" or first == "!" then
    n = string.sub(n, 2)
  end
  return n
end

local function common_prefix_len(a, b)
  local limit = math.min(#a, #b)
  local i = 1
  while i <= limit and string.sub(a, i, i) == string.sub(b, i, i) do
    i = i + 1
  end
  return i - 1
end

return function(ctx)
  if ctx.trigger ~= "join" or ctx.changed_channel == nil or ctx.changed_channel == "" then
    return nil
  end

  local changed = ctx.channels[ctx.changed_channel]
  if changed == nil then
    return nil
  end

  local assignment = {}
  for name, channel in pairs(ctx.channels) do
    assignment[name] = channel.group
  end

  local changed_name = normalize(ctx.changed_channel)
  local best_group = nil
  local best_len = 0
  local min_len = 4

  for name, channel in pairs(ctx.channels) do
    if name ~= ctx.changed_channel then
      local prefix_len = common_prefix_len(changed_name, normalize(name))
      if prefix_len > best_len then
        best_len = prefix_len
        best_group = channel.group
      end
    end
  end

  if best_group == nil or best_len < min_len then
    return nil
  end

  assignment[ctx.changed_channel] = best_group
  return assignment
end