package gateway

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// GroupRouter selects the next container from a group using a load-balancing strategy.
// Currently supports round-robin; extensible to weighted strategies.
type GroupRouter struct {
	mu       sync.Mutex
	counters map[string]*atomic.Uint64
}

// NewGroupRouter creates a new GroupRouter.
func NewGroupRouter() *GroupRouter {
	return &GroupRouter{
		counters: make(map[string]*atomic.Uint64),
	}
}

// Pick returns the next container name from the group via round-robin.
func (gr *GroupRouter) Pick(group *GroupConfig) string {
	if len(group.Containers) == 0 {
		return ""
	}
	if len(group.Containers) == 1 {
		return group.Containers[0]
	}

	gr.mu.Lock()
	counter, ok := gr.counters[group.Name]
	if !ok {
		counter = &atomic.Uint64{}
		gr.counters[group.Name] = counter
	}
	gr.mu.Unlock()

	idx := counter.Add(1) - 1
	return group.Containers[idx%uint64(len(group.Containers))]
}

// TopologicalSort returns container names in dependency-first order for a target.
// The target itself is included as the last element.
// Returns an error if cycles are detected or a dependency is missing.
func TopologicalSort(target string, allContainers []ContainerConfig) ([]string, error) {
	// Build lookup maps.
	cfgMap := make(map[string]*ContainerConfig, len(allContainers))
	for i := range allContainers {
		cfgMap[allContainers[i].Name] = &allContainers[i]
	}

	if _, ok := cfgMap[target]; !ok {
		return nil, fmt.Errorf("target container %q not found", target)
	}

	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var order []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("dependency cycle detected involving %q", name)
		}
		visiting[name] = true

		cfg, ok := cfgMap[name]
		if !ok {
			return fmt.Errorf("dependency %q not found in container list", name)
		}

		for _, dep := range cfg.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		order = append(order, name)
		return nil
	}

	if err := visit(target); err != nil {
		return nil, err
	}

	return order, nil
}
