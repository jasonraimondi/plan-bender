package schema

import "fmt"

// DetectCycles uses Kahn's algorithm to detect dependency cycles in issues.
// Returns error messages listing cycle-participant IDs.
func DetectCycles(issues []IssueYaml) []string {
	if len(issues) == 0 {
		return nil
	}

	// Build adjacency list and in-degree map from blocked_by edges.
	// Edge: blocker -> blocked (if A is blocked_by B, then B -> A)
	ids := make(map[int]bool, len(issues))
	inDegree := make(map[int]int, len(issues))
	adj := make(map[int][]int, len(issues))

	for _, iss := range issues {
		ids[iss.ID] = true
		if _, ok := inDegree[iss.ID]; !ok {
			inDegree[iss.ID] = 0
		}
	}

	for _, iss := range issues {
		for _, dep := range iss.BlockedBy {
			if ids[dep] {
				adj[dep] = append(adj[dep], iss.ID)
				inDegree[iss.ID]++
			}
		}
	}

	// BFS from zero in-degree nodes
	var queue []int
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	sorted := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if sorted == len(issues) {
		return nil
	}

	// Collect cycle participants
	var cycleIDs []int
	for id, deg := range inDegree {
		if deg > 0 {
			cycleIDs = append(cycleIDs, id)
		}
	}

	msg := "dependency cycle detected among issues:"
	for _, id := range cycleIDs {
		msg += fmt.Sprintf(" #%d", id)
	}

	return []string{msg}
}
