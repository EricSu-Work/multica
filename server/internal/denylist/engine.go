package denylist

import "sync"

// Engine holds a snapshot of active rules. The snapshot is replaced
// atomically by the loader on ConfigMap changes; concurrent Evaluate
// calls see the snapshot they read under the read lock.
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

func NewEngine(initial []Rule) *Engine {
	return &Engine{rules: initial}
}

func (e *Engine) Replace(next []Rule) {
	e.mu.Lock()
	e.rules = next
	e.mu.Unlock()
}

func (e *Engine) Evaluate(in Input) Verdict {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, r := range rules {
		hitTitle := r.TitleRegex != nil && r.TitleRegex.MatchString(in.Title)
		hitDesc := r.DescriptionRegex != nil && r.DescriptionRegex.MatchString(in.Description)
		if hitTitle || hitDesc {
			return Verdict{Blocked: true, RuleCode: r.Code, Reason: r.Description}
		}
	}
	return Verdict{Blocked: false}
}
