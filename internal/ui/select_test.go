package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hzionn/prompt-manager-cli/internal/prompt"
	"github.com/hzionn/prompt-manager-cli/internal/search"
)

func TestSelectPrompt_ValidChoice(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
	}

	input := strings.NewReader("2\n")
	var output bytes.Buffer

	selected, err := SelectPrompt(prompts, input, &output)
	if err != nil {
		t.Fatalf("SelectPrompt() error = %v", err)
	}

	if selected.Name != "beta" {
		t.Fatalf("expected beta, got %s", selected.Name)
	}

	if !strings.Contains(output.String(), "Select a prompt:") {
		t.Fatalf("expected prompt list to be written, got %q", output.String())
	}
}

func TestSelectPrompt_DefaultsToFirstOption(t *testing.T) {
	prompts := []prompt.Prompt{{Name: "alpha"}, {Name: "beta"}}
	input := strings.NewReader("\n")
	var output bytes.Buffer

	selected, err := SelectPrompt(prompts, input, &output)
	if err != nil {
		t.Fatalf("SelectPrompt() error = %v", err)
	}

	if selected.Name != "alpha" {
		t.Fatalf("expected alpha, got %s", selected.Name)
	}
}

func TestSelectPrompt_AllowsNameSelection(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
	}

	input := strings.NewReader("beta\n")
	var output bytes.Buffer

	selected, err := SelectPrompt(prompts, input, &output)
	if err != nil {
		t.Fatalf("SelectPrompt() error = %v", err)
	}

	if selected.Name != "beta" {
		t.Fatalf("expected beta, got %s", selected.Name)
	}
}

func TestSelectPrompt_AllowsPartialName(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "product-brief"},
		{Name: "brainstorm"},
	}

	input := strings.NewReader("product\n")
	var output bytes.Buffer

	selected, err := SelectPrompt(prompts, input, &output)
	if err != nil {
		t.Fatalf("SelectPrompt() error = %v", err)
	}

	if selected.Name != "product-brief" {
		t.Fatalf("expected product-brief, got %s", selected.Name)
	}
}

func TestSelectPrompt_HandleCarriageReturnOnly(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
	}

	input := strings.NewReader("beta\r")
	var output bytes.Buffer

	selected, err := SelectPrompt(prompts, input, &output)
	if err != nil {
		t.Fatalf("SelectPrompt() error = %v", err)
	}

	if selected.Name != "beta" {
		t.Fatalf("expected beta, got %s", selected.Name)
	}
}

func TestSelectPrompt_InvalidSelection(t *testing.T) {
	prompts := []prompt.Prompt{{Name: "alpha"}, {Name: "beta"}}
	input := strings.NewReader("three\n")
	var output bytes.Buffer

	_, err := SelectPrompt(prompts, input, &output)
	if err == nil {
		t.Fatal("expected error for invalid selection")
	}

	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("expected ErrInvalidSelection, got %v", err)
	}
}

func TestSelectPrompt_NoPrompts(t *testing.T) {
	input := strings.NewReader("1\n")
	var output bytes.Buffer

	_, err := SelectPrompt(nil, input, &output)
	if err == nil {
		t.Fatal("expected error when there are no prompts")
	}

	if !errors.Is(err, ErrNoPrompts) {
		t.Fatalf("expected ErrNoPrompts, got %v", err)
	}
}

func TestSelectorModelHandlesArrowNavigation(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	model := newSelectorModel(prompts, "", search.Options{}, Options{})
	if model.cursor != 0 {
		t.Fatalf("expected initial cursor at 0, got %d", model.cursor)
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(*selectorModel)
	if model.cursor != 1 {
		t.Fatalf("expected cursor to move down to 1, got %d", model.cursor)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = next.(*selectorModel)
	if model.cursor != 0 {
		t.Fatalf("expected cursor to move up to 0, got %d", model.cursor)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = next.(*selectorModel)
	if model.cursor != 0 {
		t.Fatalf("expected cursor to ignore j while typing, got %d", model.cursor)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = next.(*selectorModel)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(*selectorModel)
	if model.mode != modeNavigate {
		t.Fatal("expected modeNavigate after pressing esc")
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = next.(*selectorModel)
	if model.cursor != 1 {
		t.Fatalf("expected cursor to move down with j in navigation mode, got %d", model.cursor)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = next.(*selectorModel)
	if model.cursor != 0 {
		t.Fatalf("expected cursor to move up with k in navigation mode, got %d", model.cursor)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(*selectorModel)
	if model.mode != modeFilter {
		t.Fatal("expected esc to toggle back to typing mode")
	}
}

func TestSelectorModelFiltersWhileTyping(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "brainstorm"},
		{Name: "product-brief"},
		{Name: "code-review"},
	}

	model := newSelectorModel(prompts, "", search.Options{}, Options{})
	assertPromptNames(t, model.filtered, []string{"brainstorm", "code-review", "product-brief"})

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = next.(*selectorModel)

	if model.query != "p" {
		t.Fatalf("expected query to be %q, got %q", "p", model.query)
	}

	assertPromptNames(t, model.filtered, []string{"product-brief"})

	view := model.View()
	if !strings.Contains(view, "Filter: p") {
		t.Fatalf("expected filter to appear in view, got %q", view)
	}
	if strings.Contains(view, "brainstorm") {
		t.Fatalf("expected brainstorm to be filtered out, got %q", view)
	}
}

func TestSelectorModelBackspaceRestoresResults(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "alpine"},
	}

	model := newSelectorModel(prompts, "", search.Options{}, Options{})

	steps := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyRunes, Runes: []rune{'l'}},
		{Type: tea.KeyRunes, Runes: []rune{'p'}},
	}
	for _, msg := range steps {
		next, _ := model.Update(msg)
		model = next.(*selectorModel)
	}
	assertPromptNames(t, model.filtered, []string{"alpha", "alpine"})

	for i := 0; i < 3; i++ {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model = next.(*selectorModel)
	}

	assertPromptNames(t, model.filtered, []string{"alpha", "alpine", "beta"})
	if model.query != "" {
		t.Fatalf("expected query to be cleared, got %q", model.query)
	}
}

func TestSelectorModelInitialQuery(t *testing.T) {
	prompts := []prompt.Prompt{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "alpine"},
	}

	model := newSelectorModel(prompts, "alp", search.Options{}, Options{})
	assertPromptNames(t, model.filtered, []string{"alpha", "alpine"})

	if model.query != "alp" {
		t.Fatalf("expected initial query to persist, got %q", model.query)
	}
}

func assertPromptNames(t *testing.T, prompts []prompt.Prompt, want []string) {
	t.Helper()
	if len(prompts) != len(want) {
		t.Fatalf("expected %d prompts, got %d (%v)", len(want), len(prompts), prompts)
	}
	for i, p := range prompts {
		if p.Name != want[i] {
			t.Fatalf("expected prompt %d to be %q, got %q", i, want[i], p.Name)
		}
	}
}
