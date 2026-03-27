import { describe, it, expect, vi } from "vitest";
import { DEFAULT_CONFIG } from "../config/defaults.js";
import type { Config } from "../config/schema.js";

// Mock the Linear SDK before importing the backend
vi.mock("@linear/sdk", () => {
  const mockIssue = {
    id: "issue-1",
    identifier: "TEAM-1",
    title: "Test issue",
    description: "Test",
    url: "https://linear.app/team/issue/TEAM-1",
    priority: 2,
    state: Promise.resolve({ id: "state-1", name: "In Progress" }),
    update: vi.fn().mockResolvedValue({}),
  };

  const mockProject = {
    id: "proj-1",
    name: "Test Project",
    url: "https://linear.app/team/project/proj-1",
    issues: vi.fn().mockResolvedValue({
      nodes: [mockIssue],
    }),
  };

  class MockLinearClient {
    createProject = vi.fn().mockResolvedValue({
      project: Promise.resolve(mockProject),
    });
    createIssue = vi.fn().mockResolvedValue({
      issue: Promise.resolve(mockIssue),
    });
    issue = vi.fn().mockResolvedValue(mockIssue);
    project = vi.fn().mockResolvedValue(mockProject);
    workflowStates = vi.fn().mockResolvedValue({
      nodes: [
        { id: "state-1", name: "In Progress" },
        { id: "state-2", name: "Done" },
        { id: "state-3", name: "Backlog" },
      ],
    });
  }

  return { LinearClient: MockLinearClient };
});

// Import after mock is set up
const { LinearBackend } = await import("./linear.js");

function makeLinearConfig(): Config {
  return {
    ...DEFAULT_CONFIG,
    backend: "linear",
    linear: {
      api_key: "test-key",
      team: "team-1",
      status_map: {
        "in-progress": "In Progress",
      },
    },
  };
}

function makeIssue() {
  return {
    id: 1,
    slug: "test-issue",
    name: "Test issue",
    track: "intent" as const,
    status: "in-progress",
    priority: "high",
    points: 2,
    labels: ["AFK"],
    assignee: null,
    blocked_by: [],
    blocking: [],
    branch: null,
    pr: null,
    linear_id: "TEAM-1",
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

describe("Linear backend", () => {
  it("constructs with valid config", () => {
    expect(() => new LinearBackend(makeLinearConfig())).not.toThrow();
  });

  it("throws without api_key", () => {
    const config = {
      ...makeLinearConfig(),
      linear: { team: "team-1" },
    };
    expect(() => new LinearBackend(config)).toThrow("api_key");
  });

  it("creates a project", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.createProject({
      name: "Test",
      slug: "test",
      status: "active",
      created: "2026-03-26",
      updated: "2026-03-26",
      description: "Test",
      why: "Test",
      outcome: "Test",
    });
    expect(result.id).toBe("proj-1");
    expect(result.name).toBe("Test Project");
  });

  it("creates an issue", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.createIssue(makeIssue(), "proj-1");
    expect(result.id).toBe("TEAM-1");
    expect(result.title).toBe("Test issue");
  });

  it("updates an issue", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.updateIssue(makeIssue());
    expect(result.id).toBe("TEAM-1");
  });

  it("pulls an issue", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.pullIssue("TEAM-1");
    expect(result.id).toBe("TEAM-1");
    expect(result.title).toBe("Test issue");
  });

  it("pulls a project with issues", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.pullProject("proj-1");
    expect(result.project.name).toBe("Test Project");
    expect(result.issues.length).toBe(1);
  });

  it("maps priority correctly", async () => {
    const backend = new LinearBackend(makeLinearConfig());
    const result = await backend.pullIssue("TEAM-1");
    expect(result.priority).toBe("high"); // priority 2 → high
  });
});
