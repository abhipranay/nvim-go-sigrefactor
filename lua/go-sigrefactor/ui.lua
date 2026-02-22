local config = require("go-sigrefactor.config")
local cli = require("go-sigrefactor.cli")

local M = {}

-- State for the current refactoring session
local state = {
  bufnr = nil,
  winnr = nil,
  original_signature = nil,
  params = {},
  returns = {},
  selected_idx = 1,
  mode = "params", -- "params" or "returns"
  filename = nil,
  offset = nil,
  augroup = nil,
  in_input = false, -- Track if we're in a vim.ui.input prompt
}

-- Create floating window
local function create_float(title, content)
  local opts = config.options.ui
  local width = opts.width
  local height = opts.height

  -- Center the window
  local row = math.floor((vim.o.lines - height) / 2)
  local col = math.floor((vim.o.columns - width) / 2)

  -- Create buffer
  local bufnr = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_option(bufnr, "buftype", "nofile")
  vim.api.nvim_buf_set_option(bufnr, "bufhidden", "wipe")

  -- Create window
  local winnr = vim.api.nvim_open_win(bufnr, true, {
    relative = "editor",
    width = width,
    height = height,
    row = row,
    col = col,
    style = "minimal",
    border = opts.border,
    title = " " .. title .. " ",
    title_pos = "center",
  })

  return bufnr, winnr
end

-- Format parameter for display
local function format_param(param, idx, selected)
  local prefix = selected and ">" or " "
  local variadic = param.variadic and "..." or ""
  if param.name and param.name ~= "" then
    return string.format("%s %d. %s %s%s", prefix, idx, param.name, variadic, param.type)
  else
    return string.format("%s %d. %s%s", prefix, idx, variadic, param.type)
  end
end

-- Render the UI
local function render()
  if not state.bufnr or not vim.api.nvim_buf_is_valid(state.bufnr) then
    return
  end

  local lines = {}
  local sig = state.original_signature

  -- Header with function name
  local func_header = "func "
  if sig.receiver then
    local recv_type = sig.receiver.pointer and ("*" .. sig.receiver.type) or sig.receiver.type
    func_header = func_header .. "(" .. sig.receiver.name .. " " .. recv_type .. ") "
  end
  func_header = func_header .. sig.name

  table.insert(lines, func_header)
  table.insert(lines, "")

  -- Parameters section
  table.insert(lines, "Parameters:                    [j/k] Navigate  [J/K] Reorder")
  table.insert(lines, "                               [r] Rename  [d] Delete  [a] Add")
  table.insert(lines, "")

  for i, param in ipairs(state.params) do
    local selected = state.mode == "params" and i == state.selected_idx
    table.insert(lines, format_param(param, i, selected))
  end

  if #state.params == 0 then
    table.insert(lines, "  (no parameters)")
  end

  table.insert(lines, "")

  -- Returns section
  table.insert(lines, "Returns:                       [Tab] Switch section")
  table.insert(lines, "")

  for i, ret in ipairs(state.returns) do
    local selected = state.mode == "returns" and i == state.selected_idx
    table.insert(lines, format_param(ret, i, selected))
  end

  if #state.returns == 0 then
    table.insert(lines, "  (no return values)")
  end

  table.insert(lines, "")
  table.insert(lines, "")

  -- Action buttons
  table.insert(lines, "[p] Preview Changes    [Enter] Apply    [q/Esc] Cancel")

  vim.api.nvim_buf_set_lines(state.bufnr, 0, -1, false, lines)
end

-- Setup autocmds for the UI window
local function setup_autocmds()
  -- Create augroup for this window
  state.augroup = vim.api.nvim_create_augroup("GoSigRefactorUI", { clear = true })

  -- Close window when leaving it (focus lost)
  vim.api.nvim_create_autocmd("WinLeave", {
    group = state.augroup,
    buffer = state.bufnr,
    callback = function()
      -- Skip if we're in a vim.ui.input prompt
      if state.in_input then
        return
      end
      -- Delay close slightly to handle focus transitions
      vim.defer_fn(function()
        if state.in_input then
          return
        end
        M.close()
      end, 50)
    end,
  })

  -- Close window when buffer is hidden
  vim.api.nvim_create_autocmd("BufHidden", {
    group = state.augroup,
    buffer = state.bufnr,
    callback = function()
      M.close()
    end,
  })
end

-- Wrapper for vim.ui.input that tracks state
local function safe_input(opts, callback)
  state.in_input = true
  vim.ui.input(opts, function(input)
    state.in_input = false
    -- Refocus our window if it still exists
    if state.winnr and vim.api.nvim_win_is_valid(state.winnr) then
      vim.api.nvim_set_current_win(state.winnr)
    end
    callback(input)
  end)
end

-- Setup keymaps for the UI buffer
local function setup_keymaps()
  local opts = { buffer = state.bufnr, noremap = true, silent = true }

  -- Navigation
  vim.keymap.set("n", "j", function()
    local list = state.mode == "params" and state.params or state.returns
    if state.selected_idx < #list then
      state.selected_idx = state.selected_idx + 1
      render()
    end
  end, opts)

  vim.keymap.set("n", "k", function()
    if state.selected_idx > 1 then
      state.selected_idx = state.selected_idx - 1
      render()
    end
  end, opts)

  -- Reorder
  vim.keymap.set("n", "J", function()
    local list = state.mode == "params" and state.params or state.returns
    if state.selected_idx < #list then
      list[state.selected_idx], list[state.selected_idx + 1] =
        list[state.selected_idx + 1], list[state.selected_idx]
      state.selected_idx = state.selected_idx + 1
      render()
    end
  end, opts)

  vim.keymap.set("n", "K", function()
    local list = state.mode == "params" and state.params or state.returns
    if state.selected_idx > 1 then
      list[state.selected_idx], list[state.selected_idx - 1] =
        list[state.selected_idx - 1], list[state.selected_idx]
      state.selected_idx = state.selected_idx - 1
      render()
    end
  end, opts)

  -- Switch section
  vim.keymap.set("n", "<Tab>", function()
    if state.mode == "params" then
      state.mode = "returns"
      state.selected_idx = math.min(state.selected_idx, math.max(1, #state.returns))
    else
      state.mode = "params"
      state.selected_idx = math.min(state.selected_idx, math.max(1, #state.params))
    end
    render()
  end, opts)

  -- Rename
  vim.keymap.set("n", "r", function()
    local list = state.mode == "params" and state.params or state.returns
    if #list == 0 then return end

    local current = list[state.selected_idx]
    safe_input({ prompt = "New name: ", default = current.name or "" }, function(input)
      if input then
        current.name = input
        render()
      end
    end)
  end, opts)

  -- Delete
  vim.keymap.set("n", "d", function()
    local list = state.mode == "params" and state.params or state.returns
    if #list == 0 then return end

    table.remove(list, state.selected_idx)
    if state.selected_idx > #list and #list > 0 then
      state.selected_idx = #list
    end
    render()
  end, opts)

  -- Add
  vim.keymap.set("n", "a", function()
    safe_input({ prompt = "Parameter name: " }, function(name)
      if not name or name == "" then
        state.in_input = false
        return
      end
      safe_input({ prompt = "Parameter type: " }, function(typ)
        if not typ or typ == "" then return end

        local list = state.mode == "params" and state.params or state.returns
        table.insert(list, state.selected_idx + 1, { name = name, type = typ })
        state.selected_idx = state.selected_idx + 1
        render()
      end)
    end)
  end, opts)

  -- Preview
  vim.keymap.set("n", "p", function()
    M.preview_changes()
  end, opts)

  -- Apply
  vim.keymap.set("n", "<CR>", function()
    M.apply_changes()
  end, opts)

  -- Cancel
  vim.keymap.set("n", "q", function()
    M.close()
  end, opts)

  vim.keymap.set("n", "<Esc>", function()
    M.close()
  end, opts)
end

-- Open the signature editor
function M.open(filename, offset)
  state.filename = filename
  state.offset = offset

  cli.analyze(filename, offset, function(result, err)
    if err then
      vim.notify("Failed to analyze: " .. err, vim.log.levels.ERROR)
      return
    end

    state.original_signature = result.signature
    state.params = vim.deepcopy(result.signature.params or {})
    state.returns = vim.deepcopy(result.signature.returns or {})
    state.selected_idx = 1
    state.mode = "params"

    state.bufnr, state.winnr = create_float("Change Signature", "")
    setup_autocmds()
    setup_keymaps()
    render()
  end)
end

-- Preview changes
function M.preview_changes()
  local spec = {
    params = state.params,
    returns = state.returns,
  }

  cli.refactor(state.filename, state.offset, spec, function(result, err)
    if err then
      vim.notify("Failed to get preview: " .. err, vim.log.levels.ERROR)
      return
    end

    -- Show preview in a split
    local preview_lines = { "=== Preview of Changes ===" }
    for filename, edits in pairs(result.changes or {}) do
      table.insert(preview_lines, "")
      table.insert(preview_lines, "File: " .. filename)
      table.insert(preview_lines, string.rep("-", 50))
      for _, edit in ipairs(edits) do
        table.insert(preview_lines, string.format(
          "Line %d-%d: %s",
          edit.range.start.line,
          edit.range["end"].line,
          edit.newText:gsub("\n", "\\n")
        ))
      end
    end

    -- Create preview buffer
    local preview_buf = vim.api.nvim_create_buf(false, true)
    vim.api.nvim_buf_set_lines(preview_buf, 0, -1, false, preview_lines)
    vim.api.nvim_buf_set_option(preview_buf, "buftype", "nofile")
    vim.api.nvim_buf_set_option(preview_buf, "modifiable", false)

    -- Open in split below
    vim.cmd("botright split")
    vim.api.nvim_win_set_buf(0, preview_buf)
    vim.api.nvim_win_set_height(0, 10)

    -- Map q to close preview
    vim.keymap.set("n", "q", "<cmd>close<CR>", { buffer = preview_buf })
  end)
end

-- Apply changes
function M.apply_changes()
  local spec = {
    params = state.params,
    returns = state.returns,
  }

  -- Check for new parameters that need default values
  local orig_param_names = {}
  for _, p in ipairs(state.original_signature.params or {}) do
    if p.name then
      orig_param_names[p.name] = true
    end
  end

  local new_params = {}
  for _, p in ipairs(state.params) do
    if p.name and not orig_param_names[p.name] then
      table.insert(new_params, p)
    end
  end

  local function do_refactor(default_values)
    spec.defaultValues = default_values or {}

    cli.refactor(state.filename, state.offset, spec, function(result, err)
      if err then
        vim.notify("Refactoring failed: " .. err, vim.log.levels.ERROR)
        return
      end

      -- Apply workspace edits
      M.close()

      local edits_applied = 0
      local files_modified = 0



      for filename, file_edits in pairs(result.changes or {}) do
        -- Sort edits by offset descending
        table.sort(file_edits, function(a, b)
          return a.range.start.offset > b.range.start.offset
        end)

        -- Read file content
        local file = io.open(filename, "r")
        if file then
          local content = file:read("*a")
          file:close()

          -- Apply edits (Go offsets are 0-indexed, Lua strings are 1-indexed)
          for _, edit in ipairs(file_edits) do
            local start_offset = edit.range.start.offset
            local end_offset = edit.range["end"].offset

            -- Go offset N means Nth byte (0-indexed)
            -- Lua string position N means Nth character (1-indexed)
            -- To replace Go bytes [start, end):
            --   Keep Lua chars 1 to start (bytes 0 to start-1)
            --   Insert new text
            --   Keep Lua chars from end+1 onwards (bytes end onwards)
            content = content:sub(1, start_offset) .. edit.newText .. content:sub(end_offset + 1)
            edits_applied = edits_applied + 1
          end

          -- Write file
          file = io.open(filename, "w")
          if file then
            file:write(content)
            file:close()
            files_modified = files_modified + 1
          else
            vim.notify("Failed to write: " .. filename, vim.log.levels.ERROR)
          end
        else
          vim.notify("Failed to read: " .. filename, vim.log.levels.ERROR)
        end
      end

      vim.notify(string.format("Applied %d edits to %d files", edits_applied, files_modified), vim.log.levels.INFO)

      -- Reload affected buffers
      for filename, _ in pairs(result.changes or {}) do
        for _, buf in ipairs(vim.api.nvim_list_bufs()) do
          if vim.api.nvim_buf_get_name(buf) == filename then
            vim.api.nvim_buf_call(buf, function()
              vim.cmd("edit!")
            end)
          end
        end
      end
    end)
  end

  if #new_params > 0 then
    -- Ask for default values for new parameters
    local defaults = {}
    local function ask_next(idx)
      if idx > #new_params then
        do_refactor(defaults)
        return
      end

      local param = new_params[idx]
      safe_input({
        prompt = string.format("Default value for '%s' (%s): ", param.name, param.type)
      }, function(input)
        if input and input ~= "" then
          defaults[param.name] = input
        else
          defaults[param.name] = "nil"
        end
        ask_next(idx + 1)
      end)
    end
    ask_next(1)
  else
    do_refactor({})
  end
end

-- Close the UI
function M.close()
  -- Clean up autocmds
  if state.augroup then
    pcall(vim.api.nvim_del_augroup_by_id, state.augroup)
    state.augroup = nil
  end

  if state.winnr and vim.api.nvim_win_is_valid(state.winnr) then
    vim.api.nvim_win_close(state.winnr, true)
  end
  state.bufnr = nil
  state.winnr = nil
end

-- Check if UI is open (for external access)
function M.is_open()
  return state.winnr ~= nil and vim.api.nvim_win_is_valid(state.winnr)
end

return M
