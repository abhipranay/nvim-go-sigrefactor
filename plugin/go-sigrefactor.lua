-- Prevent loading twice
if vim.g.loaded_go_sigrefactor then
  return
end
vim.g.loaded_go_sigrefactor = true

-- Lazy load on command or Go files
vim.api.nvim_create_user_command("GoChangeSignature", function()
  require("go-sigrefactor").change_signature()
end, {
  desc = "Change Go function signature",
})
