import { defineCommand } from "citty";
import { join } from "node:path";
import { resolveConfig } from "../../config/index.js";
import { loadIssues } from "../shared.js";

const STATUS_COLORS: Record<string, string> = {
  done: "#2da44e",
  "in-progress": "#bf8700",
  "in-review": "#bf8700",
  blocked: "#cf222e",
  backlog: "#656d76",
  todo: "#656d76",
  qa: "#8250df",
  canceled: "#656d76",
};

export const graphCommand = defineCommand({
  meta: {
    name: "graph",
    description: "Output mermaid dependency graph for a plan",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug",
      required: true,
    },
  },
  async run({ args }) {
    const config = resolveConfig(process.cwd());
    const issuesDir = join(config.plans_dir, args.slug, "issues");
    const issues = loadIssues(issuesDir);

    const lines: string[] = ["graph TD"];

    // Node definitions
    for (const issue of issues) {
      const escaped = issue.name
        .replace(/"/g, "&quot;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");
      const label = `${issue.id}: ${escaped}`;
      lines.push(`  ${issue.id}["${label}"]`);
    }

    // Edges from blocked_by
    for (const issue of issues) {
      for (const dep of issue.blocked_by) {
        lines.push(`  ${dep} --> ${issue.id}`);
      }
    }

    // Style by status
    const statusGroups = new Map<string, number[]>();
    for (const issue of issues) {
      const group = statusGroups.get(issue.status) ?? [];
      group.push(issue.id);
      statusGroups.set(issue.status, group);
    }

    for (const [status, ids] of statusGroups) {
      const color = STATUS_COLORS[status] ?? "#656d76";
      for (const id of ids) {
        lines.push(`  style ${id} fill:${color},color:#fff`);
      }
    }

    console.log("```mermaid");
    console.log(lines.join("\n"));
    console.log("```");
  },
});

