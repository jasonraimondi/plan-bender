import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, mkdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml } from "yaml";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const cli = join(__dirname, "..", "..", "dist", "cli.mjs");

function makeTmpPlan(): string {
  const dir = mkdtempSync(join(tmpdir(), "pb-status-"));
  const planDir = join(dir, "plans", "test");
  const issuesDir = join(planDir, "issues");
  mkdirSync(issuesDir, { recursive: true });

  writeFileSync(
    join(planDir, "prd.yaml"),
    toYaml({
      name: "Test Plan",
      slug: "test",
      status: "active",
      created: "2026-03-26",
      updated: "2026-03-26",
      description: "Test",
      why: "Test",
      outcome: "Test",
    }),
    "utf-8",
  );

  const issues = [
    {
      id: 1, slug: "first", name: "First", track: "intent", status: "done",
      priority: "high", points: 1, labels: ["AFK"], assignee: null,
      blocked_by: [], blocking: [2], branch: null, pr: null, linear_id: null,
      created: "2026-03-26", updated: "2026-03-26", tdd: false,
      outcome: "X", scope: "X", acceptance_criteria: ["X"], steps: ["X"], use_cases: [],
    },
    {
      id: 2, slug: "second", name: "Second", track: "data", status: "backlog",
      priority: "medium", points: 2, labels: ["AFK"], assignee: null,
      blocked_by: [1], blocking: [], branch: null, pr: null, linear_id: null,
      created: "2026-03-26", updated: "2026-03-26", tdd: true,
      outcome: "Y", scope: "Y", acceptance_criteria: ["Y"], steps: ["Y"], use_cases: [],
    },
  ];

  for (const issue of issues) {
    writeFileSync(
      join(issuesDir, `${issue.id}-${issue.slug}.yaml`),
      toYaml(issue),
      "utf-8",
    );
  }

  return dir;
}

describe("status command", () => {
  it("shows all plans status", () => {
    const dir = makeTmpPlan();
    const out = execFileSync("node", [cli, "status"], {
      cwd: dir,
      encoding: "utf-8",
    });
    expect(out).toContain("Test Plan");
    expect(out).toContain("1/2 done");
  });

  it("shows detailed single-plan status", () => {
    const dir = makeTmpPlan();
    const out = execFileSync("node", [cli, "status", "test"], {
      cwd: dir,
      encoding: "utf-8",
    });
    expect(out).toContain("#1 [done]");
    expect(out).toContain("#2 [backlog]");
  });
});

describe("graph command", () => {
  it("outputs mermaid graph", () => {
    const dir = makeTmpPlan();
    const out = execFileSync("node", [cli, "graph", "test"], {
      cwd: dir,
      encoding: "utf-8",
    });
    expect(out).toContain("```mermaid");
    expect(out).toContain("graph TD");
    expect(out).toContain("1 --> 2");
    expect(out).toContain("```");
  });
});
