import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, mkdirSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml, parse as parseYaml } from "yaml";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const cli = join(__dirname, "..", "..", "dist", "cli.mjs");

function makeTmpPlan(): string {
  const dir = mkdtempSync(join(tmpdir(), "pb-sync-"));
  const planDir = join(dir, "plans", "test");
  const issuesDir = join(planDir, "issues");
  mkdirSync(issuesDir, { recursive: true });

  writeFileSync(
    join(planDir, "prd.yaml"),
    toYaml({
      name: "Test",
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

  writeFileSync(
    join(issuesDir, "1-first.yaml"),
    toYaml({
      id: 1,
      slug: "first",
      name: "First",
      track: "intent",
      status: "backlog",
      priority: "high",
      points: 1,
      labels: ["AFK"],
      assignee: null,
      blocked_by: [],
      blocking: [],
      branch: null,
      pr: null,
      linear_id: null,
      created: "2026-03-26",
      updated: "2026-03-26",
      tdd: false,
      outcome: "Done",
      scope: "X",
      acceptance_criteria: ["X"],
      steps: ["X"],
      use_cases: [],
    }),
    "utf-8",
  );

  return dir;
}

describe("sync command", () => {
  it("syncs yaml-fs backend (push is no-op for yaml-fs but runs without error)", () => {
    const dir = makeTmpPlan();
    const out = execFileSync("node", [cli, "sync", "test"], {
      cwd: dir,
      encoding: "utf-8",
      env: { ...process.env },
    });
    expect(out).toContain("Sync push:");
  });

  it("rejects non-numeric issue ID", () => {
    const dir = makeTmpPlan();
    try {
      execFileSync("node", [cli, "sync", "test#abc"], {
        cwd: dir,
        encoding: "utf-8",
        env: { ...process.env },
      });
      expect.unreachable("should have exited with error");
    } catch (err) {
      const stderr = (err as { stderr: string }).stderr;
      expect(stderr).toContain("Invalid issue ID");
      expect(stderr).toContain("must be a number");
    }
  });
});
