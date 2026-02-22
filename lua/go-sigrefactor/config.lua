local M = {}

M.defaults = {
  -- Path to gosigrefactor binary
  binary = nil, -- Will be auto-detected

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

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", M.defaults, opts or {})

  -- Auto-detect binary path
  if not M.options.binary then
    local plugin_dir = vim.fn.fnamemodify(debug.getinfo(1, "S").source:sub(2), ":h:h:h")
    local binary_path = plugin_dir .. "/bin/gosigrefactor"
    if vim.fn.filereadable(binary_path) == 1 then
      M.options.binary = binary_path
    else
      -- Try to find in PATH
      local handle = io.popen("which gosigrefactor 2>/dev/null")
      if handle then
        local result = handle:read("*a"):gsub("%s+$", "")
        handle:close()
        if result ~= "" then
          M.options.binary = result
        end
      end
    end
  end
end

return M
