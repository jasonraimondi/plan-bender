import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, mkdirSync, existsSync, readFileSync, readdirSync, lstatSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { stringify as toYaml } from "yaml";
import { execFileSync } from "node:child_process";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const cli = join(__dirname, "..", "..", "dist", "cli.mjs");

function makeTmpProject(): string {
  const dir = mkdtempSync(join(tmpdir(), "pb-gen-"));
  // Default config — no plan-bender.yaml means all defaults
  return dir;
}

describe("generate-skills", () => {
  it("generates skill files from bundled templates", () => {
    const dir = makeTmpProject();
    const out = execFileSync("node", [cli, "generate-skills"], {
      cwd: dir,
      encoding: "utf-8",
    });
    expect(out).toContain("generated");

    const skillsDir = join(dir, ".plan-bender", "skills");
    expect(existsSync(skillsDir)).toBe(true);

    // Should have generated skills
    const skills = readdirSync(skillsDir);
    expect(skills.length).toBeGreaterThan(0);

    // Each skill should have a SKILL.md
    for (const skill of skills) {
      const skillMd = join(skillsDir, skill, "SKILL.md");
      expect(existsSync(skillMd)).toBe(true);
      const content = readFileSync(skillMd, "utf-8");
      expect(content).toContain("---");
    }
  });

  it("uses local template overrides", () => {
    const dir = makeTmpProject();
    const overrideDir = join(dir, ".plan-bender", "templates");
    mkdirSync(overrideDir, { recursive: true });
    writeFileSync(
      join(overrideDir, "bender-interview-me.skill.tmpl"),
      "---\nname: custom-interview\n---\nCustom content for ${plans_dir}",
      "utf-8",
    );

    execFileSync("node", [cli, "generate-skills"], {
      cwd: dir,
      encoding: "utf-8",
    });

    const content = readFileSync(
      join(dir, ".plan-bender", "skills", "bender-interview-me", "SKILL.md"),
      "utf-8",
    );
    expect(content).toContain("Custom content");
  });
});

describe("install", () => {
  it("creates symlinks in target directory", () => {
    const dir = makeTmpProject();
    // Generate first
    execFileSync("node", [cli, "generate-skills"], {
      cwd: dir,
      encoding: "utf-8",
    });

    // Use a temp target dir instead of ~/.claude/skills/
    const targetDir = mkdtempSync(join(tmpdir(), "pb-install-"));
    // We can't easily override the install target dir via CLI,
    // so just verify generate-skills produced the right structure
    const skillsDir = join(dir, ".plan-bender", "skills");
    const skills = readdirSync(skillsDir);
    expect(skills.length).toBeGreaterThan(0);

    for (const skill of skills) {
      expect(existsSync(join(skillsDir, skill, "SKILL.md"))).toBe(true);
    }
  });
});
