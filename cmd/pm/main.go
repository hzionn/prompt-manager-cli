package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/hzionn/prompt-manager-cli/internal/clipboard"
	"github.com/hzionn/prompt-manager-cli/internal/config"
	"github.com/hzionn/prompt-manager-cli/internal/prompt"
	"github.com/hzionn/prompt-manager-cli/internal/search"
	"github.com/hzionn/prompt-manager-cli/internal/ui"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type appContext struct {
	settings   config.Settings
	configPath string
	promptOpts prompt.Options
	searchOpts search.Options
}

func newAppContext() appContext {
	configPath := config.DefaultPath()
	settings := config.Load(configPath)
	maxBytes := int64(settings.FileSystem.MaxFileSizeKB) * 1024
	return appContext{
		settings: settings,
		configPath: configPath,
		promptOpts: prompt.Options{
			Extensions:     settings.FileSystem.Extensions,
			IgnorePatterns: settings.FileSystem.IgnorePatterns,
			MaxFileSize:    maxBytes,
		},
		searchOpts: search.Options{
			MaxResults: settings.FuzzySearch.MaxResults,
		},
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	ctx := newAppContext()
	if len(args) == 0 {
		return runPick(ctx, []string{}, in, out)
	}

	switch args[0] {
	case "pick":
		return runPick(ctx, args[1:], in, out)
	case "search":
		return runSearch(ctx, args[1:], in, out)
	case "ls":
		return runList(ctx, args[1:], out)
	case "cat":
		return runCat(ctx, args[1:], out)
	case "mesh":
		return runMesh(ctx, args[1:], in, out)
	case "completion":
		return runCompletion(args[1:], out)
	case "--help", "-h", "help":
		printUsage(out)
		return nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return runPick(ctx, args, in, out)
		}
		query := strings.Join(args, " ")
		return runPickWithQuery(ctx, query, "", false, out)
	}
}

func runPick(ctx appContext, args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dirFlag string
	var query string
	var interactive bool
	var copyToClipboard bool

	fs.StringVar(&dirFlag, "dir", "", "Prompt directories (comma separated)")
	fs.StringVar(&query, "query", "", "Query to select a prompt non-interactively")
	fs.BoolVar(&interactive, "interactive", false, "Force interactive selection")
	fs.BoolVar(&copyToClipboard, "copy", false, "Copy the chosen prompt to the clipboard")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if query != "" && interactive {
		return errors.New("cannot use --query and --interactive together")
	}

	if query != "" {
		return runPickWithQuery(ctx, query, dirFlag, copyToClipboard, out)
	}

	if !interactive && fs.NArg() > 0 {
		// Allow positional query arguments.
		query = strings.Join(fs.Args(), " ")
		return runPickWithQuery(ctx, query, dirFlag, copyToClipboard, out)
	}

	return runPickInteractive(ctx, dirFlag, copyToClipboard, in, out)
}

func runPickWithQuery(ctx appContext, query, dirFlag string, copyToClipboard bool, out io.Writer) error {
	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	results := search.Search(prompts, query, ctx.searchOpts)
	if len(results) == 0 {
		return fmt.Errorf("no prompts found for query %q; prompt dirs: %s; config: %s", query, formatPromptDirs(ctx, dirFlag), ctx.configPath)
	}

	return outputPrompt(results[0].Content, copyToClipboard, out)
}

func runPickInteractive(ctx appContext, dirFlag string, copyToClipboard bool, in io.Reader, out io.Writer) error {
	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	if len(prompts) == 0 {
		return fmt.Errorf("no prompts available; prompt dirs: %s; config: %s", formatPromptDirs(ctx, dirFlag), ctx.configPath)
	}

	sorted := search.Search(prompts, "", search.Options{})
	// Use stderr for the interactive UI to keep stdout clean for the prompt output
	selected, err := ui.SelectPromptWithQuery(sorted, "", search.Options{}, ui.Options{TruncateLength: ctx.settings.UI.TruncateLength}, in, os.Stderr)
	if err != nil {
		return err
	}

	return outputPrompt(selected.Content, copyToClipboard, out)
}

type fdReader interface {
	Fd() uintptr
}

type terminalAwareReader interface {
	IsTerminal() bool
}

func shouldReadFromInput(in io.Reader) bool {
	if in == nil {
		return false
	}

	if aware, ok := in.(terminalAwareReader); ok {
		return !aware.IsTerminal()
	}

	if fd, ok := in.(fdReader); ok {
		return !term.IsTerminal(int(fd.Fd()))
	}

	return true
}

func runSearch(ctx appContext, args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dirFlag string
	var limit int
	var interactive bool
	fs.StringVar(&dirFlag, "dir", "", "Prompt directories (comma separated)")
	fs.IntVar(&limit, "limit", ctx.searchOpts.MaxResults, "Maximum number of results")
	fs.BoolVar(&interactive, "interactive", false, "Launch interactive picker with the query")

	if err := fs.Parse(args); err != nil {
		return err
	}

	queryArgs := fs.Args()
	if len(queryArgs) == 0 {
		return errors.New("search requires a query argument")
	}
	query := strings.Join(queryArgs, " ")

	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	opts := ctx.searchOpts
	if limit > 0 {
		opts.MaxResults = limit
	}

	results := search.Search(prompts, query, opts)
	if len(results) == 0 {
		return fmt.Errorf("no prompts found for query %q; prompt dirs: %s; config: %s", query, formatPromptDirs(ctx, dirFlag), ctx.configPath)
	}

	if interactive {
		// Use stderr for the interactive UI to keep stdout clean for the prompt output
		selected, err := ui.SelectPromptWithQuery(prompts, query, opts, ui.Options{TruncateLength: ctx.settings.UI.TruncateLength}, in, os.Stderr)
		if err != nil {
			return err
		}
		return writePrompt(out, selected.Content)
	}

	for _, p := range results {
		fmt.Fprintf(out, "%s\t%s\n", p.Name, p.Path)
	}
	return nil
}

func runList(ctx appContext, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dirFlag string
	fs.StringVar(&dirFlag, "dir", "", "Prompt directories (comma separated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	results := search.Search(prompts, "", search.Options{})
	for _, p := range results {
		fmt.Fprintln(out, p.Name)
	}
	return nil
}

func runCat(ctx appContext, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dirFlag string
	fs.StringVar(&dirFlag, "dir", "", "Prompt directories (comma separated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	names := fs.Args()
	if len(names) == 0 {
		return errors.New("cat requires a prompt name")
	}
	name := strings.Join(names, " ")

	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	promptItem, err := resolvePromptByQuery(prompts, name)
	if err != nil {
		return err
	}

	return writePrompt(out, promptItem.Content)
}

func runMesh(ctx appContext, args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("mesh", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dirFlag string
	fs.StringVar(&dirFlag, "dir", "", "Prompt directories (comma separated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	names := fs.Args()
	if len(names) == 0 {
		return errors.New("mesh requires at least one prompt name")
	}

	prompts, err := loadPrompts(ctx, dirFlag)
	if err != nil {
		return err
	}

	for _, name := range names {
		promptItem, err := resolvePromptByQuery(prompts, name)
		if err != nil {
			return err
		}
		if err := writePrompt(out, promptItem.Content); err != nil {
			return err
		}
		fmt.Fprintln(out)
	}

	if shouldReadFromInput(in) {
		if extra, err := io.ReadAll(in); err == nil && len(extra) > 0 {
			if err := writePrompt(out, string(extra)); err != nil {
				return err
			}
		}
	}

	return nil
}

func loadPrompts(ctx appContext, dirFlag string) ([]prompt.Prompt, error) {
	dirs := ctx.settings.DefaultDirs
	if dirFlag != "" {
		dirs = splitAndTrim(dirFlag)
	}
	return prompt.LoadFromDirs(expandDirs(dirs), ctx.promptOpts)
}

func formatPromptDirs(ctx appContext, dirFlag string) string {
	dirs := ctx.settings.DefaultDirs
	if dirFlag != "" {
		dirs = splitAndTrim(dirFlag)
	}
	dirs = expandDirs(dirs)
	if len(dirs) == 0 {
		return "(none configured)"
	}
	return strings.Join(dirs, ", ")
}

func resolvePromptByQuery(prompts []prompt.Prompt, query string) (prompt.Prompt, error) {
	if query == "" {
		return prompt.Prompt{}, errors.New("prompt name cannot be empty")
	}

	for _, p := range prompts {
		if strings.EqualFold(p.Name, query) {
			return p, nil
		}
		if aliasMatch(p, query) {
			return p, nil
		}
	}

	normalizedQuery := normalizeQuery(query)
	if normalizedQuery != "" {
		for _, p := range prompts {
			if normalizeQuery(p.Name) == normalizedQuery {
				return p, nil
			}
			if aliasMatchNormalized(p, normalizedQuery) {
				return p, nil
			}
		}
	}

	return prompt.Prompt{}, fmt.Errorf("prompt %q not found", query)
}

func splitAndTrim(input string) []string {
	parts := strings.Split(input, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, `pm - prompt manager CLI

Usage:
  pm [--query <query>] [--dir <dir>] [--copy]
  pm pick [--query <query>] [--interactive] [--copy]
  pm search [--limit N] [--interactive] <query>
  pm ls
  pm cat <name>
  pm mesh <name> [<name>...]
  pm completion <bash|zsh|fish>

Flags:
  --dir           Override prompt directories (comma separated)
  --query         Provide a query for prompt selection
  --interactive   Force interactive selection
  --copy          Copy the chosen prompt to the clipboard
  --limit         Maximum number of results for search`)
}

func runCompletion(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("completion requires a shell (bash, zsh, or fish)")
	}

	switch args[0] {
	case "bash":
		_, err := fmt.Fprint(out, bashCompletion)
		return err
	case "zsh":
		_, err := fmt.Fprint(out, zshCompletion)
		return err
	case "fish":
		_, err := fmt.Fprint(out, fishCompletion)
		return err
	default:
		return fmt.Errorf("unsupported shell %q (expected bash, zsh, or fish)", args[0])
	}
}

const bashCompletion = `# bash completion for pm
_pm_complete() {
  local cur prev
  _init_completion || return

  local commands="pick search ls cat mesh help"
  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
    return
  fi

  case ${COMP_WORDS[1]} in
    cat|mesh)
      local prompts
      prompts=$(pm ls 2>/dev/null)
      COMPREPLY=( $(compgen -W "$prompts" -- "$cur") )
      return
      ;;
  esac
}
complete -F _pm_complete pm
`

const zshCompletion = `#compdef pm

_pm() {
  local state
  local -a commands
  commands=(
    'pick:interactive picker'
    'search:search prompts'
    'ls:list prompts'
    'cat:print a prompt'
    'mesh:combine prompts'
    'help:show help'
  )

  _arguments -C \
    '1:command:->command' \
    '*:args:->args'

  case $state in
    command)
      _describe 'command' commands
      ;;
    args)
      case $words[2] in
        cat|mesh)
          local -a prompts
          prompts=("${(@f)$(pm ls 2>/dev/null)}")
          _describe 'prompt' prompts
          ;;
      esac
      ;;
  esac
}

compdef _pm pm
`

const fishCompletion = `# fish completion for pm
complete -c pm -f -n '__fish_use_subcommand' -a 'pick search ls cat mesh help'
complete -c pm -f -n '__fish_seen_subcommand_from cat mesh' -a '(pm ls 2>/dev/null)'
`

func outputPrompt(content string, copyToClipboard bool, out io.Writer) error {
	cleaned := normalizeContent(content)
	if err := writePrompt(out, cleaned); err != nil {
		return err
	}
	if copyToClipboard {
		if err := clipboard.Copy(cleaned); err != nil {
			return fmt.Errorf("copy to clipboard: %w", err)
		}
	}
	return nil
}

func normalizeContent(content string) string {
	return strings.TrimRight(content, "\r\n")
}

func writePrompt(out io.Writer, content string) error {
	_, err := fmt.Fprintln(out, normalizeContent(content))
	return err
}

func expandDirs(dirs []string) []string {
	expanded := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		expanded = append(expanded, expandTilde(dir))
	}
	return expanded
}

func expandTilde(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func normalizeQuery(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"_", " ",
		"-", " ",
		"/", " ",
		".", " ",
		",", " ",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	)
	clean := replacer.Replace(strings.ToLower(value))
	return strings.Join(strings.Fields(clean), " ")
}

func aliasMatch(p prompt.Prompt, query string) bool {
	for _, alias := range promptAliases(p) {
		if strings.EqualFold(alias, query) {
			return true
		}
	}
	return false
}

func aliasMatchNormalized(p prompt.Prompt, normalizedQuery string) bool {
	if normalizedQuery == "" {
		return false
	}
	for _, alias := range promptAliases(p) {
		if normalizeQuery(alias) == normalizedQuery {
			return true
		}
	}
	return false
}

func promptAliases(p prompt.Prompt) []string {
	if p.FrontMatter == nil {
		return nil
	}
	raw, ok := p.FrontMatter["aliases"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []string:
		return append([]string(nil), v...)
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}
