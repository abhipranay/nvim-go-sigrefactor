local M = {}

local REPO = "abhipranay/nvim-go-sigrefactor"
local BINARY_NAME = "gosigrefactor"

-- Detect OS and architecture (macOS and Linux only)
local function get_platform()
  local os_name = vim.loop.os_uname().sysname:lower()
  local arch = vim.loop.os_uname().machine

  local goos, goarch

  if os_name:match("linux") then
    goos = "linux"
  elseif os_name:match("darwin") then
    goos = "darwin"
  else
    return nil, "Unsupported OS: " .. os_name .. ". Only macOS and Linux are supported."
  end

  if arch == "x86_64" or arch == "amd64" then
    goarch = "amd64"
  elseif arch == "aarch64" or arch == "arm64" then
    goarch = "arm64"
  else
    return nil, "Unsupported architecture: " .. arch
  end

  return goos .. "-" .. goarch, nil
end

-- Get plugin directory
local function get_plugin_dir()
  local source = debug.getinfo(1, "S").source:sub(2)
  return vim.fn.fnamemodify(source, ":h:h:h")
end

-- Check if binary exists and is executable
function M.get_binary_path()
  local plugin_dir = get_plugin_dir()
  local bin_path = plugin_dir .. "/bin/" .. BINARY_NAME

  if vim.fn.executable(bin_path) == 1 then
    return bin_path
  end

  -- Check platform-specific binary
  local platform, err = get_platform()
  if platform then
    local platform_bin = plugin_dir .. "/bin/" .. BINARY_NAME .. "-" .. platform
    if vim.fn.executable(platform_bin) == 1 then
      return platform_bin
    end
  end

  -- Check in PATH
  if vim.fn.executable(BINARY_NAME) == 1 then
    return BINARY_NAME
  end

  return nil
end

-- Try to build the binary using Go
local function try_build(plugin_dir, callback)
  local make_cmd = { "make", "-C", plugin_dir, "build" }

  vim.system(make_cmd, { text = true }, function(result)
    vim.schedule(function()
      if result.code == 0 then
        callback(plugin_dir .. "/bin/" .. BINARY_NAME, nil)
      else
        callback(nil, "Build failed: " .. (result.stderr or "unknown error"))
      end
    end)
  end)
end

-- Download binary from GitHub releases
local function download_binary(plugin_dir, callback)
  local platform, err = get_platform()
  if not platform then
    callback(nil, err)
    return
  end

  vim.notify("Downloading gosigrefactor binary...", vim.log.levels.INFO)

  -- Get latest release info
  local curl_cmd = {
    "curl", "-s", "-L",
    "https://api.github.com/repos/" .. REPO .. "/releases/latest"
  }

  vim.system(curl_cmd, { text = true }, function(result)
    vim.schedule(function()
      if result.code ~= 0 then
        callback(nil, "Failed to fetch release info")
        return
      end

      local ok, release = pcall(vim.json.decode, result.stdout)
      if not ok or not release.assets then
        callback(nil, "Failed to parse release info")
        return
      end

      -- Find matching asset
      local asset_url = nil
      local asset_name = BINARY_NAME .. "-" .. platform
      for _, asset in ipairs(release.assets) do
        if asset.name == asset_name then
          asset_url = asset.browser_download_url
          break
        end
      end

      if not asset_url then
        callback(nil, "No binary available for " .. platform .. ". Please install Go and run 'make build'")
        return
      end

      -- Download binary
      local bin_dir = plugin_dir .. "/bin"
      vim.fn.mkdir(bin_dir, "p")
      local bin_path = bin_dir .. "/" .. BINARY_NAME

      local download_cmd = {
        "curl", "-s", "-L", "-o", bin_path, asset_url
      }

      vim.system(download_cmd, { text = true }, function(dl_result)
        vim.schedule(function()
          if dl_result.code ~= 0 then
            callback(nil, "Failed to download binary")
            return
          end

          -- Make executable
          vim.fn.setfperm(bin_path, "rwxr-xr-x")

          vim.notify("gosigrefactor binary installed successfully!", vim.log.levels.INFO)
          callback(bin_path, nil)
        end)
      end)
    end)
  end)
end

-- Ensure binary is available, downloading or building if necessary
function M.ensure_binary(callback)
  local bin_path = M.get_binary_path()
  if bin_path then
    callback(bin_path, nil)
    return
  end

  local plugin_dir = get_plugin_dir()

  -- Try to build first if Go is available
  if vim.fn.executable("go") == 1 then
    vim.notify("Building gosigrefactor...", vim.log.levels.INFO)
    try_build(plugin_dir, function(path, err)
      if path then
        callback(path, nil)
      else
        -- Fall back to download
        download_binary(plugin_dir, callback)
      end
    end)
  else
    -- No Go, try to download
    download_binary(plugin_dir, callback)
  end
end

return M
