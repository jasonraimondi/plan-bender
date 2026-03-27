import { describe, it, expect } from "vitest";
import { deepMerge } from "./merge.js";
import { validateConfig, ConfigValidationError } from "./validate.js";
import { DEFAULT_CONFIG } from "./defaults.js";
import type { PartialConfig, Config } from "./schema.js";

describe("deepMerge", () => {
  it("returns defaults when no layers provided", () => {
    const result = deepMerge();
    expect(result).toEqual(DEFAULT_CONFIG);
  });

  it("overrides scalar values from project layer", () => {
    const result = deepMerge({ backend: "linear" });
    expect(result.backend).toBe("linear");
    expect(result.tracks).toEqual(DEFAULT_CONFIG.tracks);
  });

  it("replaces arrays (not appends)", () => {
    const result = deepMerge({ tracks: ["frontend", "backend"] });
    expect(result.tracks).toEqual(["frontend", "backend"]);
  });

  it("deep merges nested objects", () => {
    const result = deepMerge({ linear: { api_key: "test-key" } });
    expect(result.linear.api_key).toBe("test-key");
    expect(result.linear.team).toBeUndefined();
  });

  it("applies three layers in order — last wins", () => {
    const global: PartialConfig = { backend: "linear", max_points: 5 };
    const project: PartialConfig = { max_points: 8 };
    const local: PartialConfig = { max_points: 2 };
    const result = deepMerge(global, project, local);
    expect(result.backend).toBe("linear");
    expect(result.max_points).toBe(2);
  });

  it("skips undefined layers", () => {
    const result = deepMerge(undefined, { max_points: 5 }, undefined);
    expect(result.max_points).toBe(5);
  });

  it("does not mutate defaults", () => {
    const before = structuredClone(DEFAULT_CONFIG);
    deepMerge({ tracks: ["x"] });
    expect(DEFAULT_CONFIG).toEqual(before);
  });
});

describe("validateConfig", () => {
  it("passes valid default config", () => {
    expect(() => validateConfig(DEFAULT_CONFIG)).not.toThrow();
  });

  it("rejects invalid backend", () => {
    const config = { ...DEFAULT_CONFIG, backend: "jira" as Config["backend"] };
    expect(() => validateConfig(config)).toThrow(ConfigValidationError);
  });

  it("rejects empty tracks", () => {
    const config = { ...DEFAULT_CONFIG, tracks: [] };
    expect(() => validateConfig(config)).toThrow(ConfigValidationError);
  });

  it("rejects non-integer max_points", () => {
    const config = { ...DEFAULT_CONFIG, max_points: 2.5 };
    expect(() => validateConfig(config)).toThrow(ConfigValidationError);
  });

  it("rejects zero max_points", () => {
    const config = { ...DEFAULT_CONFIG, max_points: 0 };
    expect(() => validateConfig(config)).toThrow(ConfigValidationError);
  });

  it("requires linear.api_key when backend is linear", () => {
    const config = { ...DEFAULT_CONFIG, backend: "linear" as const, linear: {} };
    const err = getValidationErrors(config);
    expect(err).toContain("linear.api_key: required when backend is linear");
    expect(err).toContain("linear.team: required when backend is linear");
  });

  it("validates custom field definitions", () => {
    const config = {
      ...DEFAULT_CONFIG,
      issue_schema: {
        custom_fields: [
          { name: "", type: "string" as const, required: true },
        ],
      },
    };
    const err = getValidationErrors(config);
    expect(err).toContain("issue_schema.custom_fields[0].name: required");
  });

  it("requires enum_values for enum fields", () => {
    const config = {
      ...DEFAULT_CONFIG,
      issue_schema: {
        custom_fields: [
          { name: "risk", type: "enum" as const, required: false, enum_values: [] },
        ],
      },
    };
    const err = getValidationErrors(config);
    expect(err).toContain(
      'issue_schema.custom_fields[0].enum_values: required when type is "enum"',
    );
  });

  it("reports multiple errors at once", () => {
    const config = {
      ...DEFAULT_CONFIG,
      backend: "bad" as Config["backend"],
      tracks: [],
      max_points: -1,
    };
    const err = getValidationErrors(config);
    expect(err.length).toBeGreaterThanOrEqual(3);
  });
});

function getValidationErrors(config: Config): string[] {
  try {
    validateConfig(config);
    return [];
  } catch (e) {
    if (e instanceof ConfigValidationError) return e.errors;
    throw e;
  }
}
