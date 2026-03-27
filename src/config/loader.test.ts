import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml } from "yaml";
import { resolveConfig } from "./index.js";
import { DEFAULT_CONFIG } from "./defaults.js";

function makeTmpProject(files: Record<string, unknown> = {}): string {
  const dir = mkdtempSync(join(tmpdir(), "pb-test-"));
  for (const [name, content] of Object.entries(files)) {
    const path = join(dir, name);
    mkdirSync(join(path, ".."), { recursive: true });
    writeFileSync(path, toYaml(content), "utf-8");
  }
  return dir;
}

describe("resolveConfig", () => {
  it("returns defaults when no config files exist", () => {
    const dir = makeTmpProject();
    const config = resolveConfig(dir);
    expect(config).toEqual(DEFAULT_CONFIG);
  });

  it("loads project config and merges over defaults", () => {
    const dir = makeTmpProject({
      "plan-bender.yaml": { max_points: 5, tracks: ["ui", "api"] },
    });
    const config = resolveConfig(dir);
    expect(config.max_points).toBe(5);
    expect(config.tracks).toEqual(["ui", "api"]);
    expect(config.backend).toBe("yaml-fs");
  });

  it("local overrides project", () => {
    const dir = makeTmpProject({
      "plan-bender.yaml": { max_points: 5 },
      "plan-bender.local.yaml": { max_points: 1 },
    });
    const config = resolveConfig(dir);
    expect(config.max_points).toBe(1);
  });
});
