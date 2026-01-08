package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/hzionn/prompt-manager-cli/internal/prompt"
	"github.com/hzionn/prompt-manager-cli/internal/search"
)

var (
	// ErrNoPrompts indicates there are no prompts to select from.
	ErrNoPrompts = errors.New("no prompts available")
	// ErrInvalidSelection indicates the provided selection could not be parsed.
	ErrInvalidSelection = errors.New("invalid selection")
)

type fd interface {
	Fd() uintptr
}

// Options configure selector rendering.
type Options struct {
	TruncateLength int
}

const defaultTruncateLength = 120

func normalizeOptions(opts Options) Options {
	if opts.TruncateLength <= 0 {
		opts.TruncateLength = defaultTruncateLength
	}
	return opts
}

// SelectPrompt presents an interactive list of prompts and returns the chosen item.
func SelectPrompt(prompts []prompt.Prompt, in io.Reader, out io.Writer) (prompt.Prompt, error) {
	return SelectPromptWithQuery(prompts, "", search.Options{}, Options{}, in, out)
}

// SelectPromptWithQuery enables interactive filtering seeded with an initial query.
func SelectPromptWithQuery(prompts []prompt.Prompt, initialQuery string, opts search.Options, uiOpts Options, in io.Reader, out io.Writer) (prompt.Prompt, error) {
	if len(prompts) == 0 {
		return prompt.Prompt{}, ErrNoPrompts
	}

	uiOpts = normalizeOptions(uiOpts)

	if isTerminal(in) && isTerminal(out) {
		selected, err := runInteractiveSelector(prompts, initialQuery, opts, uiOpts, in, out)
		if err == nil {
			return selected, nil
		}
		if errors.Is(err, ErrInvalidSelection) {
			return prompt.Prompt{}, err
		}
		// If the TUI fails for any other reason, fall back to the simple selector.
	}

	display := prompts
	if trimmed := strings.TrimSpace(initialQuery); trimmed != "" {
		if matches := search.Search(prompts, trimmed, opts); len(matches) > 0 {
			display = matches
		}
	}

	return selectPromptFallback(display, in, out)
}

func runInteractiveSelector(prompts []prompt.Prompt, initialQuery string, opts search.Options, uiOpts Options, in io.Reader, out io.Writer) (prompt.Prompt, error) {
	model := newSelectorModel(prompts, initialQuery, opts, uiOpts)

	options := []tea.ProgramOption{
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
		tea.WithoutCatchPanics(),
		tea.WithoutSignalHandler(),
	}

	prog := tea.NewProgram(model, options...)
	finalModel, err := prog.StartReturningModel()
	if err != nil {
		return prompt.Prompt{}, err
	}

	sel := finalModel.(*selectorModel)
	if sel.cancelled || len(sel.filtered) == 0 {
		return prompt.Prompt{}, ErrInvalidSelection
	}

	return sel.filtered[sel.cursor], nil
}

func selectPromptFallback(prompts []prompt.Prompt, in io.Reader, out io.Writer) (prompt.Prompt, error) {
	fmt.Fprintln(out, "Select a prompt:")
	for idx, p := range prompts {
		fmt.Fprintf(out, "%d) %s\n", idx+1, p.Name)
	}
	fmt.Fprint(out, "> ")

	reader := bufio.NewScanner(in)
	if !reader.Scan() {
		return prompts[0], nil
	}

	text := strings.TrimSpace(reader.Text())
	if text == "" {
		return prompts[0], nil
	}

	if index, err := strconv.Atoi(text); err == nil {
		if index < 1 || index > len(prompts) {
			return prompt.Prompt{}, ErrInvalidSelection
		}
		return prompts[index-1], nil
	}

	var partialMatches []prompt.Prompt
	for _, p := range prompts {
		if strings.EqualFold(p.Name, text) {
			return p, nil
		}
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(text)) {
			partialMatches = append(partialMatches, p)
		}
	}

	if len(partialMatches) > 0 {
		return partialMatches[0], nil
	}

	return prompt.Prompt{}, ErrInvalidSelection
}

func isTerminal(v any) bool {
	file, ok := v.(fd)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

type selectorModel struct {
	allPrompts []prompt.Prompt
	filtered   []prompt.Prompt
	cursor     int
	cancelled  bool
	width      int
	height     int
	ready      bool
	query      string
	filterOpts search.Options
	uiOpts     Options
	mode       selectorMode
}

type selectorMode int

const (
	modeFilter selectorMode = iota
	modeNavigate
)

func newSelectorModel(prompts []prompt.Prompt, initialQuery string, opts search.Options, uiOpts Options) *selectorModel {
	model := &selectorModel{
		allPrompts: append([]prompt.Prompt(nil), prompts...),
		filterOpts: opts,
		uiOpts:     normalizeOptions(uiOpts),
		mode:       modeFilter,
	}
	model.applyQuery(initialQuery)
	return model
}

func (m *selectorModel) Init() tea.Cmd {
	return nil
}

func (m *selectorModel) toggleMode() {
	if m.mode == modeFilter {
		m.mode = modeNavigate
		return
	}
	m.mode = modeFilter
}

func (m *selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.toggleMode()
			return m, nil
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "backspace", "delete", "ctrl+h":
			if m.mode == modeNavigate {
				return m, nil
			}
			m.backspace()
			return m, nil
		case "enter":
			if len(m.filtered) == 0 {
				m.cancelled = true
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			m.moveUp()
		case "down", "ctrl+n":
			m.moveDown()
		case "ctrl+u":
			if m.mode == modeNavigate {
				return m, nil
			}
			m.clearQuery()
		case "j":
			if m.mode == modeNavigate {
				m.moveDown()
				return m, nil
			}
		case "k":
			if m.mode == modeNavigate {
				m.moveUp()
				return m, nil
			}
		}

		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			return m.handleRunes(msg.Runes)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	}

	return m, nil
}

func (m *selectorModel) handleRunes(runes []rune) (tea.Model, tea.Cmd) {
	if len(runes) == 0 {
		return m, nil
	}

	if m.mode == modeNavigate {
		if len(runes) == 1 {
			switch runes[0] {
			case 'j', 'J':
				m.moveDown()
			case 'k', 'K':
				m.moveUp()
			}
		}
		return m, nil
	}

	if len(runes) == 1 {
		switch runes[0] {
		case 'j', 'J':
			// Interpreted as literal input while in typing mode.
		case 'k', 'K':
			// Interpreted as literal input while in typing mode.
		}
	}

	if m.query == "" && isDigits(runes) {
		if idx, err := strconv.Atoi(string(runes)); err == nil {
			if idx >= 1 && idx <= len(m.filtered) {
				m.cursor = idx - 1
			}
		}
		return m, nil
	}

	m.applyQuery(m.query + string(runes))
	return m, nil
}

func (m *selectorModel) backspace() {
	if m.query == "" {
		return
	}
	runes := []rune(m.query)
	m.applyQuery(string(runes[:len(runes)-1]))
}

func (m *selectorModel) clearQuery() {
	if m.query == "" {
		return
	}
	m.applyQuery("")
}

func (m *selectorModel) applyQuery(query string) {
	m.query = query
	trimmed := strings.TrimSpace(query)

	if trimmed == "" {
		m.filtered = append([]prompt.Prompt(nil), m.allPrompts...)
		sortPrompts(m.filtered)
	} else {
		opts := m.filterOpts
		if opts.MaxResults <= 0 || opts.MaxResults > len(m.allPrompts) {
			opts.MaxResults = len(m.allPrompts)
		}
		m.filtered = search.Search(m.allPrompts, trimmed, opts)
	}

	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}

	if trimmed != "" {
		m.cursor = 0
	}
}

func (m *selectorModel) moveUp() {
	if len(m.filtered) == 0 {
		return
	}
	if m.cursor == 0 {
		m.cursor = len(m.filtered) - 1
		return
	}
	m.cursor--
}

func (m *selectorModel) moveDown() {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor = (m.cursor + 1) % len(m.filtered)
}

func (m *selectorModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	var b strings.Builder
	b.WriteString("\n Filter: " + m.query + "\n")
	if m.mode == modeFilter {
		b.WriteString(" Typing mode (Esc to switch to navigation). ↑/↓ move, Enter confirms, Ctrl+C cancels\n\n")
	} else {
		b.WriteString(" Navigation mode (Esc to switch to typing). ↑/↓/j/k move, Enter confirms, Ctrl+C cancels\n\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString("  No matches. Keep typing or press Esc to cancel.\n")
		return b.String()
	}

	listMax := height - 10
	if listMax < 3 {
		listMax = 3
	}
	start, end := visibleRange(len(m.filtered), m.cursor, listMax)

	for i := start; i < end; i++ {
		p := m.filtered[i]
		line := fmt.Sprintf("  %s", renderPromptTitle(p, width, m.uiOpts.TruncateLength))
		if i == m.cursor {
			line = highlight(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(renderPreview(m.filtered[m.cursor], width, m.uiOpts.TruncateLength))
	b.WriteByte('\n')

	return b.String()
}

func sortPrompts(prompts []prompt.Prompt) {
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Name < prompts[j].Name
	})
}

func visibleRange(total, cursor, maxItems int) (int, int) {
	if total <= maxItems {
		return 0, total
	}
	if cursor < 0 {
		cursor = 0
	}
	start := cursor - maxItems/2
	if start < 0 {
		start = 0
	}
	if start+maxItems > total {
		start = total - maxItems
	}
	return start, start + maxItems
}

func isDigits(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	for _, r := range runes {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func renderPromptTitle(p prompt.Prompt, width int, truncateLength int) string {
	limit := width - 4
	if truncateLength > 0 && truncateLength < limit {
		limit = truncateLength
	}
	name := truncate(p.Name, limit)
	if len(p.Tags) == 0 {
		return name
	}
	tagLine := strings.Join(p.Tags, ", ")
	full := fmt.Sprintf("%s  [%s]", name, tagLine)
	return truncate(full, limit)
}

func renderPreview(p prompt.Prompt, width int, truncateLength int) string {
	width = max(width-2, 40)

	var sections []string

	if summary := frontMatterString(p.FrontMatter, "summary"); summary != "" {
		sections = append(sections, "Summary:\n"+indent(wrap(limitText(summary, truncateLength), width), "  "))
	}

	if len(p.Tags) > 0 {
		sections = append(sections, "Tags: "+limitText(strings.Join(p.Tags, ", "), truncateLength))
	}

	if preview := snippet(p.Content, 5); preview != "" {
		sections = append(sections, "Preview:\n"+indent(wrap(limitText(preview, truncateLength), width), "  "))
	}

	return strings.Join(sections, "\n\n")
}

func frontMatterString(front map[string]any, key string) string {
	if front == nil {
		return ""
	}

	value, ok := front[key]
	if !ok || value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func wrap(text string, limit int) string {
	if limit <= 0 {
		return text
	}

	var (
		lines  []string
		words  = strings.Fields(text)
		cursor string
	)

	for _, word := range words {
		if len(cursor) == 0 {
			cursor = word
			continue
		}

		if displayWidth(cursor+" "+word) > limit {
			lines = append(lines, cursor)
			cursor = word
		} else {
			cursor += " " + word
		}
	}

	if cursor != "" {
		lines = append(lines, cursor)
	}

	return strings.Join(lines, "\n")
}

func indent(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func snippet(content string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return ""
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "…")
	}

	return strings.Join(lines, "\n")
}

func truncate(text string, length int) string {
	if length <= 0 {
		return ""
	}

	if displayWidth(text) <= length {
		return text
	}

	runes := []rune(text)
	if len(runes) <= length {
		return text
	}

	if length <= 1 {
		return "…"
	}

	return string(runes[:length-1]) + "…"
}

func limitText(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	return truncate(text, limit)
}

func highlight(text string) string {
	return "\x1b[38;5;213m" + text + "\x1b[0m"
}

func displayWidth(text string) int {
	return utf8.RuneCountInString(stripANSI(text))
}

func stripANSI(text string) string {
	result := make([]rune, 0, len(text))
	skip := false
	for _, r := range text {
		if r == '\x1b' {
			skip = true
			continue
		}
		if skip {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				skip = false
			}
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
