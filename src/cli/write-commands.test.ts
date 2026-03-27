import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, existsSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml } from "yaml";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const cli = join(__dirname, "..", "..", "dist", "cli.js");

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "pb-write-"));
}

const validPrd = toYaml({
  name: "Test",
  slug: "test",
  status: "active",
  created: "2026-03-26",
  updated: "2026-03-26",
  description: "A test PRD",
  why: "Testing",
  outcome: "Tests pass",
});

const validIssue = toYaml({
  id: 1,
  slug: "first-issue",
  name: "First issue",
  track: "intent",
  status: "backlog",
  priority: "high",
  points: 2,
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
  outcome: "Something done",
  scope: "Do the thing",
  acceptance_criteria: ["It works"],
  steps: ["Step 1"],
  use_cases: [],
});

describe("write-prd", () => {
  it("writes valid PRD from file arg", () => {
    const dir = makeTmpDir();
    const inputFile = join(dir, "input.yaml");
    writeFileSync(inputFile, validPrd, "utf-8");

    execFileSync("node", [cli, "write-prd", "test", "--file", inputFile], {
      cwd: dir,
      encoding: "utf-8",
    });

    const outPath = join(dir, "plans", "test", "prd.yaml");
    expect(existsSync(outPath)).toBe(true);
    expect(readFileSync(outPath, "utf-8")).toBe(validPrd);
  });

  it("rejects invalid PRD", () => {
    const dir = makeTmpDir();
    const inputFile = join(dir, "bad.yaml");
    writeFileSync(inputFile, toYaml({ slug: "x" }), "utf-8");

    expect(() =>
      execFileSync("node", [cli, "write-prd", "test", "--file", inputFile], {
        cwd: dir,
        encoding: "utf-8",
      }),
    ).toThrow();
  });
});

describe("write-issue", () => {
  it("writes valid issue from file arg", () => {
    const dir = makeTmpDir();
    const inputFile = join(dir, "issue.yaml");
    writeFileSync(inputFile, validIssue, "utf-8");

    execFileSync(
      "node",
      [cli, "write-issue", "test", "1", "--file", inputFile],
      { cwd: dir, encoding: "utf-8" },
    );

    const outPath = join(dir, "plans", "test", "issues", "1-first-issue.yaml");
    expect(existsSync(outPath)).toBe(true);
  });

  it("rejects issue with invalid track", () => {
    const dir = makeTmpDir();
    const inputFile = join(dir, "bad.yaml");
    writeFileSync(
      inputFile,
      toYaml({
        id: 1,
        slug: "bad",
        name: "Bad",
        track: "nope",
        status: "backlog",
        priority: "high",
        points: 1,
        labels: [],
        assignee: null,
        blocked_by: [],
        blocking: [],
        branch: null,
        pr: null,
        linear_id: null,
        created: "2026-03-26",
        updated: "2026-03-26",
        tdd: false,
        outcome: "X",
        scope: "X",
        acceptance_criteria: ["X"],
        steps: ["X"],
        use_cases: [],
      }),
      "utf-8",
    );

    expect(() =>
      execFileSync(
        "node",
        [cli, "write-issue", "test", "1", "--file", inputFile],
        { cwd: dir, encoding: "utf-8" },
      ),
    ).toThrow();
  });
});
