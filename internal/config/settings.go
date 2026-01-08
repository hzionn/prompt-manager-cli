package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Settings represents persisted configuration for the CLI.
type Settings struct {
	DefaultDirs []string            `toml:"default_dir"`
	CacheDir    string              `toml:"cache_dir"`
	FileSystem  FileSystemSettings  `toml:"file_system"`
	FuzzySearch FuzzySearchSettings `toml:"fuzzy_search"`
	UI          UISettings          `toml:"ui"`
}

// FileSystemSettings describe filesystem discovery behaviour.
type FileSystemSettings struct {
	Extensions     []string `toml:"extensions"`
	IgnorePatterns []string `toml:"ignore_patterns"`
	MaxFileSizeKB  int      `toml:"max_file_size_kb"`
}

// FuzzySearchSettings describe search behaviour.
type FuzzySearchSettings struct {
	MaxResults int `toml:"max_results"`
}

// UISettings contains UI defaults.
type UISettings struct {
	TruncateLength int `toml:"truncate_length"`
}

type rawSettings struct {
	DefaultDirs interface{}         `toml:"default_dir"`
	CacheDir    string              `toml:"cache_dir"`
	FileSystem  FileSystemSettings  `toml:"file_system"`
	FuzzySearch FuzzySearchSettings `toml:"fuzzy_search"`
	UI          UISettings          `toml:"ui"`
}

// DefaultPath returns the default configuration path for this CLI.
func DefaultPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "pmc", "settings.toml")
	}
	return "config/settings.toml"
}

// Load reads settings from the provided path. Missing or malformed files fall back to defaults.
func Load(path string) Settings {
	defaults := Settings{
		DefaultDirs: []string{"~/prompts"},
		CacheDir:    "~/.cache/pmc",
		FileSystem: FileSystemSettings{
			Extensions:     []string{".md", ".txt"},
			IgnorePatterns: []string{".DS_Store"},
			MaxFileSizeKB:  128,
		},
		FuzzySearch: FuzzySearchSettings{MaxResults: 20},
		UI:          UISettings{TruncateLength: 120},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}

	var raw rawSettings
	if err := toml.Unmarshal(data, &raw); err != nil {
		return defaults
	}

	settings := defaults

	// Handle default_dir which can be string or []string
	defaultDirs := parseStringOrSlice(raw.DefaultDirs)
	if len(defaultDirs) > 0 {
		settings.DefaultDirs = defaultDirs
	}

	if raw.CacheDir != "" {
		settings.CacheDir = raw.CacheDir
	}
	if len(raw.FileSystem.Extensions) > 0 {
		settings.FileSystem.Extensions = raw.FileSystem.Extensions
	}
	if len(raw.FileSystem.IgnorePatterns) > 0 {
		settings.FileSystem.IgnorePatterns = raw.FileSystem.IgnorePatterns
	}
	if raw.FileSystem.MaxFileSizeKB > 0 {
		settings.FileSystem.MaxFileSizeKB = raw.FileSystem.MaxFileSizeKB
	}
	if raw.FuzzySearch.MaxResults > 0 {
		settings.FuzzySearch.MaxResults = raw.FuzzySearch.MaxResults
	}
	if raw.UI.TruncateLength > 0 {
		settings.UI.TruncateLength = raw.UI.TruncateLength
	}

	return settings
}

// parseStringOrSlice handles values that can be either a string or an array of strings.
func parseStringOrSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return val
	}

	return nil
}
