# Prompt Manager CLI

> This project is still in development. Installation and settings are not convenient yet.

A fast, minimal CLI tool for managing and accessing your AI prompt library. prompt-manager-cli lets you organize, search, and reuse prompts efficiently with fuzzy search and interactive selection.

## Motivation

As a frequent user of Obsidian, I wanted a seamless way to manage my personal prompt files directly within my knowledge base. With the growing popularity of terminal-based AI tools like codex-cli, gemini-cli, and claude-code, I saw an opportunity to bridge the gap between modern prompt engineering and efficient, text-based workflows.

This project was created to make it easy to organize, access, and use prompts from text-files, while leveraging the power and flexibility of the command line. My goal is to help users streamline their workflow, keep their prompts well-organized, and take advantage of the latest AI tools—all from a familiar environment.

## Features

- 🔍 **Fuzzy Search** - Quickly find prompts with intelligent fuzzy matching
- 🎯 **Interactive Selection** - Beautiful TUI for browsing and selecting prompts
- 📋 **Multiple Commands** - Flexible CLI with `pick`, `search`, `ls`, `cat`, and `mesh` commands
- ⚙️ **Configurable** - Customize file extensions, directories, and search limits via `settings.toml`
- 📁 **Multi-Directory Support** - Load prompts from multiple directories
- 📋 **Clipboard Integration** - Copy selected prompts directly to clipboard
- 🚀 **Fast & Lightweight** - Single binary, no dependencies to install

## Installation

### From Source

Make sure you have [Go 1.24+](https://golang.org/doc/install) installed.

```bash
git clone https://github.com/hzionn/prompt-manager-cli.git
cd prompt-manager-cli
go install ./cmd/pm
```

This installs `pm` into `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset). Ensure that directory is on your `PATH`.

Copy the starter config and point the prompt directory to your desired location. Multiple directories are supported.

```bash
mkdir -p ~/.config/pmc
cp config/settings.toml ~/.config/pmc/settings.toml
```

### Quick Start

```bash
# Verify installation
pm --help
```

### Shell Completions

Optional, but recommended for tab-completion of subcommands and prompt names.

Generate completion scripts:

```bash
pm completion zsh
pm completion bash
pm completion fish
```

Enable zsh completions by adding this line to `~/.zshrc` (requires `pm` on your `PATH`):

```bash
source <(pm completion zsh)
```

## Usage

### Basic Commands

#### Pick (Default)

Launch the interactive prompt picker:

```bash
pm
```

Or pick a prompt by query without interaction:

```bash
pm "code review"
pm --query "code review"
```

#### Search

Find prompts matching a query and display matches:

```bash
pm search "code review"
pm search --limit 5 "code"
```

Use `--interactive` flag to launch the picker after search:

```bash
pm search --interactive "code"
```

#### List

Show all available prompts:

```bash
pm ls
```

#### Cat

Display a specific prompt by name (exact match, alias, normalized name, or fuzzy match):

```bash
pm cat "code review"
```

#### Mesh

Combine multiple prompts together (each name supports the same matching rules as `cat`):

```bash
pm mesh "prompt1" "prompt2"
```

You can also pipe additional content:

```bash
pm mesh "system-prompt" "context-prompt" < user-input.txt
```

### Global Flags

- `--dir <paths>` - Override default prompt directories (comma-separated)
- `--query <query>` - Provide a query for non-interactive selection
- `--copy` - Copy the chosen prompt to clipboard
- `--interactive` - Force interactive selection mode
- `--limit <n>` - Limit search results (search only)

### Examples

```bash
# Pick a prompt interactively
pm

# Search for prompts about "testing"
pm search testing

# Get a specific prompt by name
pm cat "code-review"

# Search with a limit and use interactive picker
pm search --limit 10 --interactive "review"

# Copy a prompt to clipboard
pm --query "code review" --copy

# Use prompts from custom directories
pm --dir "/path/to/prompts,~/my-prompts" search "api"

# Combine multiple prompts
pm mesh "system-prompt" "user-prompt" | pbcopy
```

## Configuration

prompt-manager-cli reads configuration from `~/.config/pmc/settings.toml`. Start from `config/settings.toml` in the repo, then edit to customize behavior:

```toml
# Default directories where prompts are stored
default_dir = ["~/prompts"]

# Cache directory for temporary data
cache_dir = "~/.cache/pmc"

# File system settings
[file_system]
# File extensions to look for when scanning directories
extensions = [".md", ".txt"]

# Patterns to ignore when scanning
ignore_patterns = [".DS_Store"]

# Maximum file size to load (in KB)
max_file_size_kb = 128

# Fuzzy search configuration
[fuzzy_search]
# Maximum number of search results to return
max_results = 20

# UI configuration
[ui]
# Maximum length to truncate prompt display
truncate_length = 120
```

### Configuration Options

| Option                         | Type         | Description                                      |
| ------------------------------ | ------------ | ------------------------------------------------ |
| `default_dir`                  | Array/String | Directories to scan for prompts                  |
| `file_system.extensions`       | Array        | File extensions to include (e.g., `.md`, `.txt`) |
| `file_system.ignore_patterns`  | Array        | Glob patterns to exclude                         |
| `file_system.max_file_size_kb` | Number       | Maximum file size to load                        |
| `fuzzy_search.max_results`     | Number       | Max search results returned                      |
| `ui.truncate_length`           | Number       | Display truncation length                        |

## Project Structure

```
prompt-manager-cli/
├── cmd/pm/
│   └── main.go              # CLI entrypoint
├── internal/
│   ├── clipboard/           # Clipboard operations
│   ├── config/              # Configuration loading
│   ├── prompt/              # Prompt loading and management
│   ├── search/              # Fuzzy search implementation
│   └── ui/                  # Interactive TUI
├── config/
│   └── settings.toml        # Default configuration
├── prompts/                 # Example prompts
├── testdata/                # Test fixtures
├── go.mod                   # Go module definition
└── README.md                # This file
```

## Development

### Building from Source

```bash
# Build the binary
go build ./cmd/pm

# Run tests
go test ./...

# Format code
find . -name '*.go' | xargs gofmt -w

# Run tests with coverage
go test -cover ./...

# Run a specific test
go test -run TestName ./...

# Format code
find . -name '*.go' | xargs gofmt -w
```

## Prompt Library Structure

Organize your prompts in directories with appropriate file extensions:

```
prompts/
├── code-review.md
├── brainstorm.txt
├── product-brief.md
└── stock_researcher.md
```

Prompt files can be in Markdown (`.md`) or text (`.txt`) format. The filename (without extension) becomes the prompt's name for selection.

### Example Prompt File

```markdown
# Code Review Checklist

- Verify naming conventions
- Ensure tests cover critical paths
- Confirm error handling
- Check for edge cases
```

## Potential Roadmap

- [ ] Prompt tags and metadata
- [ ] Better UIUX
- [ ] Expose to package managers
- [ ] Color schemes and themes
- [ ] Batch operations on multiple prompts
- [ ] Plugin system for custom search strategies

## Support

Found a bug or have a feature request? Please open an [issue](https://github.com/hzionn/prompt-manager-cli/issues).
