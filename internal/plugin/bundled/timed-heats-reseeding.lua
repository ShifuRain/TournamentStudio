local function tiers_for_group(group, k)
  local n = #group
  local tier_size = math.floor(n / k)
  local remainder = n % k
  local tiers = {}
  local idx = 1
  for j = 0, k - 1 do
    tiers[j] = {}
    local size = tier_size
    if j == k - 1 then
      size = tier_size + remainder
    end
    for _ = 1, size do
      table.insert(tiers[j], group[idx])
      idx = idx + 1
    end
  end
  return tiers
end

local function next_round_groups(groups)
  local k = #groups
  local tiers_by_group = {}
  for i = 1, k do
    tiers_by_group[i] = tiers_for_group(groups[i], k)
  end

  local new_groups = {}
  for n = 0, k - 1 do
    local new_group = {}
    for i = 1, k do
      local tier_index = (n + (i - 1)) % k
      for _, entry in ipairs(tiers_by_group[i][tier_index]) do
        table.insert(new_group, entry.team_id)
      end
    end
    new_groups[n + 1] = new_group
  end

  return new_groups
end

local function division_cuts(ranked_teams, cuts)
  -- Replaced with the real implementation in the next task.
  return {}
end

return {
  id = "timed-heats-reseeding",
  compatible_sports = {"dragonboat"},
  next_round_groups = next_round_groups,
  division_cuts = division_cuts,
}
