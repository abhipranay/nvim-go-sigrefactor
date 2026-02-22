# nvim-go-sigrefactor

A Neovim plugin providing IntelliJ-like **"Change Signature"** refactoring for Go with full semantic accuracy.

## Features

- **Reorder parameters** - Change the order of function parameters, automatically updating all call sites
- **Rename parameters** - Rename parameters with automatic updates to usages within function body
- **Add parameters** - Add new parameters with default values at call sites
- **Remove parameters** - Remove parameters and their corresponding arguments at call sites
- **Interface propagation** - Automatically update interface definitions and all implementations
- **Cross-package refactoring** - Finds and updates callers across your entire Go module
- **Test file support** - Updates `_test.go` files correctly

## Requirements

- Neovim 0.9+
- Go 1.21+

## Installation

### Using lazy.nvim

```lua
{
  "abhipranay/nvim-go-sigrefactor",
  ft = "go",
  build = "make build",
  config = function()
    require("go-sigrefactor").setup({
      keymaps = {
        change_signature = "<leader>cs",
      },
    })
  end,
}
```

### Using packer.nvim

```lua
use {
  "abhipranay/nvim-go-sigrefactor",
  ft = "go",
  run = "make build",
  config = function()
    require("go-sigrefactor").setup()
  end,
}
```

### Manual Installation

```bash
git clone https://github.com/abhipranay/nvim-go-sigrefactor.git ~/.local/share/nvim/site/pack/plugins/start/nvim-go-sigrefactor
cd ~/.local/share/nvim/site/pack/plugins/start/nvim-go-sigrefactor
make build
```

Add to your `init.lua`:

```lua
require("go-sigrefactor").setup()
```

## Usage

1. Place cursor on a function or method name
2. Run `:GoChangeSignature` or press `<leader>cs`
3. Use the interactive UI to modify the signature
4. Press `Enter` to apply or `q`/`Esc` to cancel

### UI Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate parameters |
| `J` / `K` | Reorder parameter (move down/up) |
| `r` | Rename parameter |
| `d` | Delete parameter |
| `a` | Add new parameter |
| `Tab` | Switch between parameters and returns |
| `p` | Preview changes |
| `Enter` | Apply refactoring |
| `q` / `Esc` | Cancel |

## Configuration

```lua
require("go-sigrefactor").setup({
  -- Path to gosigrefactor binary (auto-detected by default)
  binary = nil,

  -- Keymaps
  keymaps = {
    change_signature = "<leader>cs",  -- Set to false to disable
  },

  -- UI options
  ui = {
    border = "rounded",  -- "single", "double", "rounded", "solid", "shadow"
    width = 60,
    height = 20,
  },
})
```

## Commands

| Command | Description |
|---------|-------------|
| `:GoChangeSignature` | Open signature editor at cursor |
| `:GoChangeSignatureClose` | Close signature editor (if stuck) |

## How It Works

The plugin consists of two components:

1. **Go CLI Tool (`gosigrefactor`)** - Performs semantic analysis using Go's type system
2. **Neovim Lua Plugin** - Provides the interactive UI

The CLI tool uses `golang.org/x/tools/go/packages` to load type-checked packages, ensuring accurate refactoring across your entire codebase.

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed technical documentation.

## Contributing

Contributions are welcome! Please read [ARCHITECTURE.md](ARCHITECTURE.md) to understand how the plugin works before submitting PRs.

```bash
# Run tests
make test

# Build binary
make build

# Run linter
make lint
```

## License

MIT
