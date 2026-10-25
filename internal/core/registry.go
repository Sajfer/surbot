package core

import (
	"fmt"
	"sort"
	"sync"
)

type entry struct {
	cmd   Command
	owner string
}

type registry struct {
	mu       sync.RWMutex
	commands map[string]entry
}

func newRegistry() *registry {
	return &registry{commands: make(map[string]entry)}
}

func (r *registry) add(owner string, cmd Command) error {
	if cmd.Name == "" {
		return fmt.Errorf("plugin %s: command name is empty", owner)
	}
	if cmd.Handler == nil {
		return fmt.Errorf("plugin %s: command %q has no handler", owner, cmd.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.commands[cmd.Name]; ok {
		return fmt.Errorf("plugin %s: command %q is already registered by %s", owner, cmd.Name, existing.owner)
	}
	r.commands[cmd.Name] = entry{cmd: cmd, owner: owner}
	return nil
}

func (r *registry) lookup(name string) (entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.commands[name]
	return e, ok
}

func (r *registry) sorted() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cmds := make([]Command, 0, len(r.commands))
	for _, e := range r.commands {
		cmds = append(cmds, e.cmd)
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds
}
