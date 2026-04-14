package gateway

import (
	"fmt"
	"strings"
)

// mermaidID converts a container name to a valid Mermaid node identifier
// by replacing hyphens and dots with underscores.
func mermaidID(name string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(name)
}

// dockerStatusToClass maps a Docker container status string to one of the
// four Mermaid CSS class names: running, stopped, starting, failed.
func dockerStatusToClass(dockerStatus string) string {
	switch dockerStatus {
	case "running":
		return "running"
	case "restarting":
		return "starting"
	case "dead", "removing":
		return "failed"
	default:
		return "stopped"
	}
}

// buildMermaidGraph generates a Mermaid graph LR definition from gateway config
// and a live status map (containerName → docker status string).
// statusMap may be nil; missing entries default to "stopped".
func buildMermaidGraph(containers []ContainerConfig, groups []GroupConfig, statusMap map[string]string) string {
	var b strings.Builder

	b.WriteString("graph LR\n")
	b.WriteString("  classDef running fill:#1a4731,stroke:#238636,color:#3fb950\n")
	b.WriteString("  classDef stopped fill:#21262d,stroke:#30363d,color:#8b949e\n")
	b.WriteString("  classDef starting fill:#3d2e0a,stroke:#d29922,color:#e3b341\n")
	b.WriteString("  classDef failed fill:#3b1219,stroke:#da3633,color:#f85149\n")

	if len(containers) == 0 && len(groups) == 0 {
		return b.String()
	}
	b.WriteString("\n")

	// Track which containers belong to a group (rendered inline inside subgraph).
	grouped := make(map[string]bool)
	for _, g := range groups {
		for _, c := range g.Containers {
			grouped[c] = true
		}
	}

	// Subgraphs.
	for _, g := range groups {
		fmt.Fprintf(&b, "  subgraph grp_%s[\"%s\"]\n", mermaidID(g.Name), g.Name)
		for _, member := range g.Containers {
			fmt.Fprintf(&b, "    %s\n", renderNode(member, statusMap))
		}
		b.WriteString("  end\n\n")
	}

	// Ungrouped nodes.
	for _, c := range containers {
		if !grouped[c.Name] {
			fmt.Fprintf(&b, "  %s\n", renderNode(c.Name, statusMap))
		}
	}

	// Edges.
	b.WriteString("\n")
	for _, c := range containers {
		for _, dep := range c.DependsOn {
			fmt.Fprintf(&b, "  %s --> %s\n", mermaidID(c.Name), mermaidID(dep))
		}
	}

	// Click directives (requires securityLevel: 'loose' in Mermaid init).
	b.WriteString("\n")
	for _, c := range containers {
		fmt.Fprintf(&b, "  click %s onNodeClick\n", mermaidID(c.Name))
	}

	return b.String()
}

// renderNode returns a single Mermaid node line: id["label"]:::class
func renderNode(name string, statusMap map[string]string) string {
	status := ""
	if statusMap != nil {
		status = statusMap[name]
	}
	return fmt.Sprintf("%s[\"%s\"]:::%s", mermaidID(name), name, dockerStatusToClass(status))
}
