import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { render } from "../engine/index.js";
import { DEFAULT_CONFIG } from "../config/defaults.js";
import { buildTemplateContext } from "./context.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const templatesDir = join(__dirname, "..", "..", "templates");

const context = buildTemplateContext(DEFAULT_CONFIG);

const templateFiles = readdirSync(templatesDir).filter((f: string) =>
  f.endsWith(".skill.tmpl"),
);

describe("planning skill templates", () => {
  for (const file of templateFiles) {
    it(`${file} renders with expected structure`, () => {
      const content = readFileSync(join(templatesDir, file), "utf-8");
      const result = render(content, context);
      expect(result.length).toBeGreaterThan(50);
      expect(result).toContain("---");
      // Rendered templates should contain the skill name from the filename
      const skillName = file.replace(".skill.tmpl", "");
      expect(result.toLowerCase()).toContain(skillName.replace(/-/g, " ").substring(0, 8));
    });
  }

  it("prd-to-issues template includes configured tracks", () => {
    const content = readFileSync(
      join(templatesDir, "planning-prd-to-issues.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, context);
    for (const track of DEFAULT_CONFIG.tracks) {
      expect(result).toContain(track);
    }
  });

  it("prd-to-issues template includes workflow states", () => {
    const content = readFileSync(
      join(templatesDir, "planning-prd-to-issues.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, context);
    expect(result).toContain(DEFAULT_CONFIG.workflow_states.join(" → "));
  });

  it("prd-to-issues template includes step pattern", () => {
    const content = readFileSync(
      join(templatesDir, "planning-prd-to-issues.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, context);
    expect(result).toContain(DEFAULT_CONFIG.step_pattern);
  });

  it("templates adapt to custom tracks", () => {
    const customCtx = buildTemplateContext({
      ...DEFAULT_CONFIG,
      tracks: ["frontend", "backend", "infra"],
    });
    const content = readFileSync(
      join(templatesDir, "planning-prd-to-issues.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, customCtx);
    expect(result).toContain("frontend");
    expect(result).toContain("backend");
    expect(result).toContain("infra");
    // Track table should use custom tracks, not defaults
    expect(result).toContain("| `frontend` |");
    expect(result).not.toContain("| `resilience` |");
  });

  it("backend sync instructions appear for non-yaml-fs", () => {
    const linearCtx = buildTemplateContext({
      ...DEFAULT_CONFIG,
      backend: "linear",
      linear: { api_key: "test", team: "test" },
    });
    const content = readFileSync(
      join(templatesDir, "planning-write-an-issue.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, linearCtx);
    expect(result).toContain("plan-bender sync");
  });

  it("backend sync instructions hidden for yaml-fs", () => {
    const content = readFileSync(
      join(templatesDir, "planning-write-an-issue.skill.tmpl"),
      "utf-8",
    );
    const result = render(content, context);
    expect(result).not.toContain("plan-bender sync");
  });
});
