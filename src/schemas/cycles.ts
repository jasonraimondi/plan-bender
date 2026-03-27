import type { IssueYaml } from "./issue.js";

export function detectCycles(issues: IssueYaml[]): string[] {
  const ids = new Set(issues.map((i) => i.id));

  // Edge: dep → issue (dep must finish before issue can start)
  const inDeg = new Map<number, number>();
  const graph = new Map<number, number[]>();

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

  // Kahn's algorithm
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

  const cycleNodes = [...ids].filter((id) => !sorted.includes(id));
  return [
    `Dependency cycle detected involving issues: ${cycleNodes.map((id) => `#${id}`).join(", ")}`,
  ];
}
