import { describe, it, expect } from "vitest";
import { createBackend, registerBackend } from "./factory.js";
import type { TrackingBackend } from "./interface.js";
import { DEFAULT_CONFIG } from "../config/defaults.js";

const mockBackend: TrackingBackend = {
  async createProject() {
    return { id: "p1", name: "test" };
  },
  async createIssue() {
    return { id: "i1", title: "test", status: "open" };
  },
  async updateIssue() {
    return { id: "i1", title: "test", status: "open" };
  },
  async pullIssue() {
    return { id: "i1", title: "test", status: "open" };
  },
  async pullProject() {
    return { project: { id: "p1", name: "test" }, issues: [] };
  },
};

describe("backend factory", () => {
  it("returns registered backend", () => {
    registerBackend("yaml-fs", () => mockBackend);
    const backend = createBackend(DEFAULT_CONFIG);
    expect(backend).toBe(mockBackend);
  });

  it("throws on unknown backend", () => {
    expect(() =>
      createBackend({ ...DEFAULT_CONFIG, backend: "jira" as "yaml-fs" }),
    ).toThrow(/Unknown backend "jira"/);
  });
});
