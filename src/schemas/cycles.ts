import type { IssueYaml } from "./issue.js";

export function detectCycles(issues: IssueYaml[]): string[] {
  const adj = new Map<number, number[]>();
  const ids = new Set<number>();

  for (const issue of issues) {
    ids.add(issue.id);
    adj.set(issue.id, issue.blocked_by ?? []);
  }

  // Kahn's algorithm for topological sort
  const inDegree = new Map<number, number>();
  for (const id of ids) inDegree.set(id, 0);

  for (const [, deps] of adj) {
    for (const dep of deps) {
      if (ids.has(dep)) {
        inDegree.set(dep, (inDegree.get(dep) ?? 0) + 1);
      }
    }
  }

  // Wait — blocked_by means "this issue depends on those". So edges go:
  // issue → blocked_by (issue depends on blocked_by being done first)
  // For cycle detection with Kahn's, we need: for each issue, its blocked_by
  // are its predecessors. Edge direction: blocked_by → issue.
  // In-degree of issue = number of things that depend on it (appear in others' blocked_by).

  // Let me redo this correctly:
  // Edge: dep → issue (dep must finish before issue can start)
  const inDeg = new Map<number, number>();
  const graph = new Map<number, number[]>(); // dep → [issues that depend on dep]

  for (const id of ids) {
    inDeg.set(id, 0);
    graph.set(id, []);
  }

  for (const issue of issues) {
    inDeg.set(issue.id, (issue.blocked_by ?? []).filter((d) => ids.has(d)).length);
    for (const dep of issue.blocked_by ?? []) {
      if (ids.has(dep)) {
        graph.get(dep)!.push(issue.id);
      }
    }
  }

  const queue: number[] = [];
  for (const [id, deg] of inDeg) {
    if (deg === 0) queue.push(id);
  }

  const sorted: number[] = [];
  while (queue.length > 0) {
    const node = queue.shift()!;
    sorted.push(node);
    for (const next of graph.get(node) ?? []) {
      const newDeg = inDeg.get(next)! - 1;
      inDeg.set(next, newDeg);
      if (newDeg === 0) queue.push(next);
    }
  }

  if (sorted.length === ids.size) return [];

  // Find nodes involved in cycles
  const cycleNodes = [...ids].filter((id) => !sorted.includes(id));
  return [
    `Dependency cycle detected involving issues: ${cycleNodes.map((id) => `#${id}`).join(", ")}`,
  ];
}
