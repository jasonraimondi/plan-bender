import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml } from "yaml";
import { validatePrd } from "./prd.js";
import { validateIssue } from "./issue.js";
import { validateCrossRefs } from "./cross-refs.js";
import { detectCycles } from "./cycles.js";
import { validatePlan } from "./validate.js";
import { DEFAULT_CONFIG } from "../config/defaults.js";
import type { IssueYaml, PrdYaml } from "./types.js";

const config = DEFAULT_CONFIG;

function makeIssue(overrides: Partial<IssueYaml> = {}): IssueYaml {
  return {
    id: 1,
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
    outcome: "Something useful",
    scope: "Do the thing",
    acceptance_criteria: ["It works"],
    steps: ["Step 1"],
    use_cases: [],
    ...overrides,
  };
}

function makePrd(overrides: Partial<PrdYaml> = {}): PrdYaml {
  return {
    name: "Test",
    slug: "test",
    status: "active",
    created: "2026-03-26",
    updated: "2026-03-26",
    description: "A test PRD",
    why: "Because testing",
    outcome: "Better tests",
    ...overrides,
  };
}

describe("validatePrd", () => {
  it("passes valid PRD", () => {
    const result = validatePrd(makePrd(), "prd.yaml");
    expect(result.errors).toEqual([]);
  });

  it("rejects missing required fields", () => {
    const result = validatePrd({ slug: "x" }, "prd.yaml");
    expect(result.errors.length).toBeGreaterThan(0);
    expect(result.errors.some((e) => e.includes("name"))).toBe(true);
  });

  it("rejects invalid status", () => {
    const result = validatePrd(makePrd({ status: "bogus" }), "prd.yaml");
    expect(result.errors.some((e) => e.includes("status"))).toBe(true);
  });
});

describe("validateIssue", () => {
  it("passes valid issue", () => {
    const result = validateIssue(makeIssue(), "issue.yaml", config);
    expect(result.errors).toEqual([]);
  });

  it("rejects invalid track", () => {
    const result = validateIssue(
      makeIssue({ track: "nope" }),
      "issue.yaml",
      config,
    );
    expect(result.errors.some((e) => e.includes("track"))).toBe(true);
  });

  it("rejects points above max", () => {
    const result = validateIssue(
      makeIssue({ points: 99 }),
      "issue.yaml",
      config,
    );
    expect(result.errors.some((e) => e.includes("points"))).toBe(true);
  });

  it("rejects zero points", () => {
    const result = validateIssue(
      makeIssue({ points: 0 }),
      "issue.yaml",
      config,
    );
    expect(result.errors.some((e) => e.includes("points"))).toBe(true);
  });

  it("validates custom fields", () => {
    const customConfig = {
      ...config,
      issue_schema: {
        custom_fields: [
          { name: "team", type: "string" as const, required: true },
          {
            name: "risk",
            type: "enum" as const,
            required: false,
            enum_values: ["low", "medium", "high"],
          },
        ],
      },
    };
    const result = validateIssue(makeIssue(), "issue.yaml", customConfig);
    expect(result.errors.some((e) => e.includes("team"))).toBe(true);
  });

  it("validates enum custom field values", () => {
    const customConfig = {
      ...config,
      issue_schema: {
        custom_fields: [
          {
            name: "risk",
            type: "enum" as const,
            required: true,
            enum_values: ["low", "medium", "high"],
          },
        ],
      },
    };
    const issue = makeIssue();
    (issue as Record<string, unknown>).risk = "extreme";
    const result = validateIssue(issue, "issue.yaml", customConfig);
    expect(result.errors.some((e) => e.includes("risk"))).toBe(true);
  });
});

describe("validateCrossRefs", () => {
  it("detects missing blocked_by references", () => {
    const issues = [makeIssue({ id: 1, blocked_by: [99], blocking: [] })];
    const errors = validateCrossRefs(makePrd(), issues);
    expect(errors.some((e) => e.includes("#99"))).toBe(true);
  });

  it("detects blocked_by/blocking asymmetry", () => {
    const issues = [
      makeIssue({ id: 1, blocked_by: [2], blocking: [] }),
      makeIssue({ id: 2, blocked_by: [], blocking: [] }), // should list 1 in blocking
    ];
    const errors = validateCrossRefs(makePrd(), issues);
    expect(errors.some((e) => e.includes("does not list"))).toBe(true);
  });

  it("detects invalid use_case references", () => {
    const prd = makePrd({ use_cases: [{ id: "UC-1", description: "test" }] });
    const issues = [makeIssue({ id: 1, use_cases: ["UC-99"] })];
    const errors = validateCrossRefs(prd, issues);
    expect(errors.some((e) => e.includes("UC-99"))).toBe(true);
  });

  it("passes valid cross-references", () => {
    const prd = makePrd({ use_cases: [{ id: "UC-1", description: "test" }] });
    const issues = [
      makeIssue({ id: 1, blocked_by: [], blocking: [2], use_cases: ["UC-1"] }),
      makeIssue({ id: 2, blocked_by: [1], blocking: [], use_cases: [] }),
    ];
    const errors = validateCrossRefs(prd, issues);
    expect(errors).toEqual([]);
  });
});

describe("detectCycles", () => {
  it("returns empty for acyclic graph", () => {
    const issues = [
      makeIssue({ id: 1, blocked_by: [] }),
      makeIssue({ id: 2, blocked_by: [1] }),
      makeIssue({ id: 3, blocked_by: [2] }),
    ];
    expect(detectCycles(issues)).toEqual([]);
  });

  it("detects simple cycle", () => {
    const issues = [
      makeIssue({ id: 1, blocked_by: [2] }),
      makeIssue({ id: 2, blocked_by: [1] }),
    ];
    const result = detectCycles(issues);
    expect(result.length).toBe(1);
    expect(result[0]).toContain("#1");
    expect(result[0]).toContain("#2");
  });

  it("detects transitive cycle", () => {
    const issues = [
      makeIssue({ id: 1, blocked_by: [3] }),
      makeIssue({ id: 2, blocked_by: [1] }),
      makeIssue({ id: 3, blocked_by: [2] }),
    ];
    const result = detectCycles(issues);
    expect(result.length).toBe(1);
  });
});

describe("validatePlan (integration)", () => {
  function makeTmpPlan(prd: unknown, issues: Record<string, unknown>[]): string {
    const dir = mkdtempSync(join(tmpdir(), "pb-plan-"));
    const planDir = join(dir, "plans", "test");
    const issuesDir = join(planDir, "issues");
    mkdirSync(issuesDir, { recursive: true });
    writeFileSync(join(planDir, "prd.yaml"), toYaml(prd), "utf-8");
    for (const [i, issue] of issues.entries()) {
      writeFileSync(
        join(issuesDir, `${i + 1}-test.yaml`),
        toYaml(issue),
        "utf-8",
      );
    }
    return planDir;
  }

  it("validates a correct plan", () => {
    const planDir = makeTmpPlan(makePrd(), [makeIssue()]);
    const result = validatePlan("test", config, planDir);
    expect(result.valid).toBe(true);
  });

  it("reports PRD errors", () => {
    const planDir = makeTmpPlan({ slug: "bad" }, []);
    const result = validatePlan("test", config, planDir);
    expect(result.valid).toBe(false);
    expect(result.prd.errors.length).toBeGreaterThan(0);
  });

  it("reports issue errors", () => {
    const planDir = makeTmpPlan(makePrd(), [
      makeIssue({ points: 99 }),
    ]);
    const result = validatePlan("test", config, planDir);
    expect(result.valid).toBe(false);
  });
});
