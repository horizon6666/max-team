package tool

import "sync"

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) ForAgent(allowedNames []string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allowed := make(map[string]bool, len(allowedNames))
	for _, name := range allowedNames {
		allowed[name] = true
	}

	var result []Tool
	for name, t := range r.tools {
		if allowed[name] {
			result = append(result, t)
		}
	}
	return result
}

func RegisterBuiltins(r *Registry, projectRoot string, sandbox bool) {
	r.Register(&ReadFile{projectRoot: projectRoot, sandbox: sandbox})
	r.Register(&WriteFile{projectRoot: projectRoot, sandbox: sandbox})
	r.Register(&ListDir{projectRoot: projectRoot, sandbox: sandbox})
	r.Register(&RunShell{})
	r.Register(&GitDiff{projectRoot: projectRoot})
	r.Register(&GitCommit{projectRoot: projectRoot})
	r.Register(&GitStatus{projectRoot: projectRoot})
	r.Register(&SearchCode{projectRoot: projectRoot})
}
