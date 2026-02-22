local M = {}

M.defaults = {
  -- Path to gosigrefactor binary
  binary = nil, -- Will be auto-detected/downloaded

  -- Keymaps
  keymaps = {
    change_signature = "<leader>cs",
  },

  -- UI options
  ui = {
    border = "rounded",
    width = 60,
    height = 20,
  },
}

M.options = {}
M.binary_ready = false

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", M.defaults, opts or {})

  -- If binary explicitly provided, use it
  if M.options.binary and vim.fn.executable(M.options.binary) == 1 then
    M.binary_ready = true
    return
  end

  -- Otherwise, ensure binary is available (build or download)
  local binary = require("go-sigrefactor.binary")
  binary.ensure_binary(function(path, err)
    if path then
      M.options.binary = path
      M.binary_ready = true
    else
      vim.notify("go-sigrefactor: " .. (err or "Failed to get binary"), vim.log.levels.ERROR)
    end
  end)
end

-- Get binary path, ensuring it's ready
function M.get_binary()
  if M.options.binary and vim.fn.executable(M.options.binary) == 1 then
    return M.options.binary
  end

  local binary = require("go-sigrefactor.binary")
  return binary.get_binary_path()
end

return M
