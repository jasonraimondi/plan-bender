import { describe, it, expect, beforeEach } from "vitest";
import { mkdtempSync, readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { parse as parseYaml } from "yaml";
import { DEFAULT_CONFIG } from "../config/defaults.js";
import type { Config } from "../config/schema.js";
import type { PrdYaml } from "../schemas/prd.js";
import type { IssueYaml } from "../schemas/issue.js";
import "./yaml-fs.js"; // side-effect: registers backend
import { createBackend } from "./factory.js";

function makeConfig(plansDir: string): Config {
  return { ...DEFAULT_CONFIG, plans_dir: plansDir };
}

function makePrd(): PrdYaml {
  return {
    name: "Test",
    slug: "test",
    status: "active",
    created: "2026-03-26",
    updated: "2026-03-26",
    description: "Test PRD",
    why: "Testing",
    outcome: "Tests pass",
  };
}

function makeIssue(id = 1): IssueYaml {
  return {
    id,
    slug: "test-issue",
    name: "Test issue",
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
    outcome: "Done",
    scope: "Do it",
    acceptance_criteria: ["Works"],
    steps: ["Step 1"],
    use_cases: [],
  };
}

describe("yaml-fs backend", () => {
  let plansDir: string;
  let config: Config;

  beforeEach(() => {
    plansDir = mkdtempSync(join(tmpdir(), "pb-yamlfs-"));
    config = makeConfig(plansDir);
  });

  it("creates via factory", () => {
    const backend = createBackend(config);
    expect(backend).toBeDefined();
  });

  it("createProject writes prd.yaml and creates issues dir", async () => {
    const backend = createBackend(config);
    const result = await backend.createProject(makePrd());
    expect(result.id).toBe("test");
    expect(existsSync(join(plansDir, "test", "prd.yaml"))).toBe(true);
    expect(existsSync(join(plansDir, "test", "issues"))).toBe(true);
  });

  it("createIssue writes issue YAML", async () => {
    const backend = createBackend(config);
    await backend.createProject(makePrd());
    const result = await backend.createIssue(makeIssue(), "test");
    expect(result.id).toBe("1");
    expect(result.title).toBe("Test issue");

    const path = join(plansDir, "test", "issues", "1-test-issue.yaml");
    expect(existsSync(path)).toBe(true);
    const written = parseYaml(readFileSync(path, "utf-8")) as IssueYaml;
    expect(written.name).toBe("Test issue");
  });

  it("updateIssue updates existing YAML", async () => {
    const backend = createBackend(config);
    await backend.createProject(makePrd());
    await backend.createIssue(makeIssue(), "test");

    const updated = { ...makeIssue(), status: "in-progress" };
    await backend.updateIssue(updated);

    const path = join(plansDir, "test", "issues", "1-test-issue.yaml");
    const written = parseYaml(readFileSync(path, "utf-8")) as IssueYaml;
    expect(written.status).toBe("in-progress");
  });

  it("pullIssue reads issue data", async () => {
    const backend = createBackend(config);
    await backend.createProject(makePrd());
    await backend.createIssue(makeIssue(), "test");

    const result = await backend.pullIssue("test/1");
    expect(result.id).toBe("1");
    expect(result.title).toBe("Test issue");
    expect(result.status).toBe("backlog");
  });

  it("pullProject reads all issues", async () => {
    const backend = createBackend(config);
    await backend.createProject(makePrd());
    await backend.createIssue(makeIssue(1), "test");
    await backend.createIssue({ ...makeIssue(2), slug: "second-issue", name: "Second" }, "test");

    const result = await backend.pullProject("test");
    expect(result.project.name).toBe("Test");
    expect(result.issues.length).toBe(2);
  });
});
