package denylist

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sync"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type yamlFile struct {
	SchemaVersion int `yaml:"schema_version"`
	Rules         []struct {
		Code               string `yaml:"code"`
		Description        string `yaml:"description"`
		TitlePattern       string `yaml:"title_pattern"`
		DescriptionPattern string `yaml:"description_pattern"`
		CaseInsensitive    bool   `yaml:"case_insensitive"`
	} `yaml:"rules"`
}

// Load reads the deny-list YAML at path and returns parsed rules. Skips
// individual rules whose regex fails to compile or that have no pattern
// at all (fail-open per rule). Returns an error if the file itself can't
// be read or has unsupported schema_version.
func Load(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("denylist: read %s: %w", path, err)
	}
	var f yamlFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("denylist: parse %s: %w", path, err)
	}
	if f.SchemaVersion != 1 {
		return nil, fmt.Errorf("denylist: unsupported schema_version %d", f.SchemaVersion)
	}
	rules := make([]Rule, 0, len(f.Rules))
	for _, raw := range f.Rules {
		titleR, err := compileMaybe(raw.TitlePattern, raw.CaseInsensitive)
		if err != nil {
			slog.Warn("denylist: skip rule (bad title_pattern)", "code", raw.Code, "err", err)
			continue
		}
		descR, err := compileMaybe(raw.DescriptionPattern, raw.CaseInsensitive)
		if err != nil {
			slog.Warn("denylist: skip rule (bad description_pattern)", "code", raw.Code, "err", err)
			continue
		}
		if titleR == nil && descR == nil {
			slog.Warn("denylist: skip rule (no pattern)", "code", raw.Code)
			continue
		}
		rules = append(rules, Rule{
			Code:             raw.Code,
			Description:      raw.Description,
			TitleRegex:       titleR,
			DescriptionRegex: descR,
		})
	}
	return rules, nil
}

func compileMaybe(pattern string, ci bool) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	src := pattern
	if ci {
		src = "(?i)" + src
	}
	return regexp.Compile(src)
}

// Watch loads the YAML at path and starts a goroutine that reloads it on
// file modification (fsnotify Write|Create). Returns the engine and a
// stop function for graceful shutdown.
func Watch(path string) (*Engine, func() error, error) {
	initial, err := Load(path)
	if err != nil {
		return nil, nil, err
	}
	engine := NewEngine(initial)
	slog.Info("denylist: loaded rules", "path", path, "count", len(initial))

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return engine, func() error { return nil }, fmt.Errorf("denylist: fsnotify: %w", err)
	}
	if err := w.Add(path); err != nil {
		_ = w.Close()
		return engine, func() error { return nil }, fmt.Errorf("denylist: watch %s: %w", path, err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					next, err := Load(path)
					if err != nil {
						slog.Error("denylist: reload failed (keeping previous rules)", "err", err)
						continue
					}
					engine.Replace(next)
					slog.Info("denylist: reloaded", "count", len(next))
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Error("denylist: watch error", "err", err)
			}
		}
	}()

	stopFn := func() error {
		close(stop)
		err := w.Close()
		wg.Wait()
		return err
	}
	return engine, stopFn, nil
}
