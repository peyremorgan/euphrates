-- Group channels by a shared stem before the first '-', '_' or '.'.
--
-- Examples:
--   #python-help, #python-dev, #python-jobs
--   #project.api, #project.web
--
-- The strategy only acts on joins. If no compatible stem is found, it
-- delegates to the next strategy.

local function normalize(name)
  local n = string.lower(name)
  local first = string.sub(n, 1, 1)
  if first == "#" or first == "&" or first == "+" or first == "!" then
    n = string.sub(n, 2)
  end
  return n
end

local function stem(name)
  local n = normalize(name)
  local pos = string.find(n, "[-_.]")
  if pos == nil or pos <= 1 then
    return nil
  end
  local out = string.sub(n, 1, pos - 1)
  if #out < 3 then
    return nil
  end
  return out
end

return function(ctx)
  if ctx.trigger ~= "join" or ctx.changed_channel == nil or ctx.changed_channel == "" then
    return nil
  end

  local changed = ctx.channels[ctx.changed_channel]
  if changed == nil then
    return nil
  end

  local changed_stem = stem(ctx.changed_channel)
  if changed_stem == nil then
    return nil
  end

  local assignment = {}
  for name, channel in pairs(ctx.channels) do
    assignment[name] = channel.group
  end

  local best_group = nil
  local best_join_order = nil
  for name, channel in pairs(ctx.channels) do
    if name ~= ctx.changed_channel and stem(name) == changed_stem then
      if best_join_order == nil or channel.join_order < best_join_order then
        best_join_order = channel.join_order
        best_group = channel.group
      end
    end
  end

  if best_group == nil then
    return nil
  end

  assignment[ctx.changed_channel] = best_group
  return assignment
end