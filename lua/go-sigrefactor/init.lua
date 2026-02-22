local config = require("go-sigrefactor.config")
local ui = require("go-sigrefactor.ui")

local M = {}

-- Get byte offset at cursor position
local function get_cursor_offset()
  local cursor = vim.api.nvim_win_get_cursor(0)
  local line = cursor[1]
  local col = cursor[2]

  -- Calculate byte offset
  local offset = 0
  for i = 1, line - 1 do
    local line_content = vim.api.nvim_buf_get_lines(0, i - 1, i, false)[1]
    offset = offset + #line_content + 1 -- +1 for newline
  end
  offset = offset + col

  return offset
end

-- Main entry point: change signature at cursor
function M.change_signature()
  local filename = vim.fn.expand("%:p")
  local offset = get_cursor_offset()

  if vim.bo.filetype ~= "go" then
    vim.notify("Not a Go file", vim.log.levels.WARN)
    return
  end

  ui.open(filename, offset)
end

-- Close the signature editor (if open)
function M.close()
  ui.close()
end

-- Setup the plugin
function M.setup(opts)
  config.setup(opts)

  -- Create user command
  vim.api.nvim_create_user_command("GoChangeSignature", function()
    M.change_signature()
  end, {
    desc = "Change Go function signature",
  })

  -- Create close command (works globally)
  vim.api.nvim_create_user_command("GoChangeSignatureClose", function()
    M.close()
  end, {
    desc = "Close signature editor window",
  })

  -- Setup keymaps if enabled
  if config.options.keymaps.change_signature then
    vim.api.nvim_create_autocmd("FileType", {
      pattern = "go",
      callback = function()
        vim.keymap.set("n", config.options.keymaps.change_signature, function()
          M.change_signature()
        end, {
          buffer = true,
          desc = "Change Go function signature",
        })
      end,
    })
  end
end

return M
