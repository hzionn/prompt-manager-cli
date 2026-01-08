package prompt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Prompt represents a prompt file and its derived metadata.
type Prompt struct {
	Name        string
	Path        string
	Content     string
	FrontMatter map[string]any
	Tags        []string
}

// Options configure prompt discovery.
type Options struct {
	Extensions     []string
	IgnorePatterns []string
	MaxFileSize    int64 // bytes
}

// LoadFromDirs discovers prompt files under the provided directories using the supplied options.
func LoadFromDirs(dirs []string, opts Options) ([]Prompt, error) {
	var prompts []Prompt
	seen := make(map[string]struct{})

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if d.IsDir() {
				if shouldIgnore(path, opts.IgnorePatterns) {
					return filepath.SkipDir
				}
				return nil
			}

			if shouldIgnore(path, opts.IgnorePatterns) {
				return nil
			}

			if len(opts.Extensions) > 0 && !hasAllowedExtension(path, opts.Extensions) {
				return nil
			}

			if opts.MaxFileSize > 0 && d.Type().IsRegular() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				if info.Size() > opts.MaxFileSize {
					return nil
				}
			}

			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			if _, ok := seen[absPath]; ok {
				return nil
			}

			fileBytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			prompt, err := buildPrompt(path, fileBytes)
			if err != nil {
				return err
			}

			prompts = append(prompts, prompt)
			seen[absPath] = struct{}{}

			return nil
		})

		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return prompts, nil
}

func shouldIgnore(filePath string, patterns []string) bool {
	base := filepath.Base(filePath)
	slashPath := filepath.ToSlash(filePath)
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return true
		}
		normalizedPattern := filepath.ToSlash(pattern)
		matched, err = path.Match(normalizedPattern, slashPath)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func hasAllowedExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, allowed := range extensions {
		if strings.ToLower(allowed) == ext {
			return true
		}
	}
	return false
}

func buildPrompt(path string, data []byte) (Prompt, error) {
	frontMatter, content := parseFrontMatter(data)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	tags := extractTags(frontMatter)

	return Prompt{
		Name:        name,
		Path:        path,
		Content:     content,
		FrontMatter: frontMatter,
		Tags:        tags,
	}, nil
}

func parseFrontMatter(data []byte) (map[string]any, string) {
	reader := bufio.NewReader(bytes.NewReader(data))

	firstLine, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, string(data)
	}

	if strings.TrimSpace(firstLine) != "---" {
		return nil, string(data)
	}

	var buf strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			return nil, string(data)
		}

		if strings.TrimSpace(line) == "---" {
			break
		}

		buf.WriteString(line)
	}

	raw := buf.String()
	if strings.TrimSpace(raw) == "" {
		rest, _ := io.ReadAll(reader)
		content := strings.TrimLeft(string(rest), "\r\n")
		return nil, content
	}

	var front map[string]any
	if err := yaml.Unmarshal([]byte(raw), &front); err != nil {
		// If parsing fails, fall back to treating the data as raw content.
		return nil, string(data)
	}

	rest, _ := io.ReadAll(reader)
	content := strings.TrimLeft(string(rest), "\r\n")

	return normalizeFrontMatter(front), content
}

func normalizeFrontMatter(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	normalized := make(map[string]any, len(input))
	for key, value := range input {
		normalized[strings.TrimSpace(key)] = value
	}
	return normalized
}

func extractTags(front map[string]any) []string {
	if front == nil {
		return nil
	}

	raw, ok := front["tags"]
	if !ok || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case string:
		return splitAndClean(v)
	case []string:
		return cleanSlice(v)
	case []any:
		var tags []string
		for _, item := range v {
			tags = append(tags, splitAndClean(fmt.Sprint(item))...)
		}
		return unique(tags)
	default:
		return splitAndClean(fmt.Sprint(v))
	}
}

func splitAndClean(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})

	return cleanSlice(parts)
}

func cleanSlice(values []string) []string {
	var cleaned []string
	seen := make(map[string]struct{})
	for _, value := range values {
		tag := strings.TrimSpace(value)
		if tag == "" {
			continue
		}
		tagLower := strings.ToLower(tag)
		if _, ok := seen[tagLower]; ok {
			continue
		}
		seen[tagLower] = struct{}{}
		cleaned = append(cleaned, tag)
	}
	return cleaned
}

func unique(values []string) []string {
	return cleanSlice(values)
}
