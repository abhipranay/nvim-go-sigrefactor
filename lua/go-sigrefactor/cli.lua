local config = require("go-sigrefactor.config")

local M = {}

-- Execute CLI command and return parsed JSON
function M.execute(cmd, args, callback)
  local binary = config.options.binary
  if not binary then
    callback(nil, "gosigrefactor binary not found")
    return
  end

  local full_args = { binary, cmd }
  for _, arg in ipairs(args) do
    table.insert(full_args, arg)
  end

  vim.system(full_args, { text = true }, function(result)
    vim.schedule(function()
      if result.code ~= 0 then
        callback(nil, result.stderr or "Unknown error")
        return
      end

      local ok, parsed = pcall(vim.json.decode, result.stdout)
      if not ok then
        callback(nil, "Failed to parse JSON: " .. result.stdout)
        return
      end

      callback(parsed, nil)
    end)
  end)
end

-- Analyze signature at current cursor position
function M.analyze(filename, offset, callback)
  M.execute("analyze", {
    "--file=" .. filename,
    "--offset=" .. tostring(offset),
  }, callback)
end

-- Find all usages
function M.usages(filename, offset, callback)
  M.execute("usages", {
    "--file=" .. filename,
    "--offset=" .. tostring(offset),
  }, callback)
end

-- Apply refactoring
function M.refactor(filename, offset, spec, callback)
  local spec_json = vim.json.encode(spec)
  M.execute("refactor", {
    "--file=" .. filename,
    "--offset=" .. tostring(offset),
    "--spec=" .. spec_json,
  }, callback)
end

return M
