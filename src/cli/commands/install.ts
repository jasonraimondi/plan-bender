import { defineCommand } from "citty";
import {
  readdirSync,
  readFileSync,
  existsSync,
  symlinkSync,
  unlinkSync,
  lstatSync,
  mkdirSync,
  appendFileSync,
} from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";
import { resolveConfig } from "../../config/index.js";

function resolveTargetDir(installTarget: string, projectRoot: string): string {
  if (installTarget === "project") {
    return join(projectRoot, ".claude", "skills");
  }
  return join(homedir(), ".claude", "skills");
}

export const installCommand = defineCommand({
  meta: {
    name: "install",
    description: "Symlink generated skills to Claude skills directory",
  },
  async run() {
    const projectRoot = process.cwd();
    const config = resolveConfig(projectRoot);
    const sourceDir = join(projectRoot, ".plan-bender", "skills");
    const targetDir = resolveTargetDir(config.install_target, projectRoot);

    if (!existsSync(sourceDir)) {
      console.error(
        "No generated skills found. Run `plan-bender generate-skills` first.",
      );
      process.exit(1);
    }

    mkdirSync(targetDir, { recursive: true });

    let count = 0;
    for (const name of readdirSync(sourceDir)) {
      const source = join(sourceDir, name);
      const target = join(targetDir, name);

      // Update existing symlink
      if (existsSync(target)) {
        const stat = lstatSync(target);
        if (stat.isSymbolicLink()) {
          unlinkSync(target);
        } else {
          console.warn(`  skipped: ${name} (not a symlink, won't overwrite)`);
          continue;
        }
      }

      symlinkSync(source, target, "dir");
      console.log(`  installed: ${name} → ${target}`);
      count++;
    }

    if (config.install_target === "project") {
      ensureGitignore(projectRoot);
    }

    console.log(`\n${count} skills installed to ${targetDir}`);
  },
});

const GITIGNORE_ENTRY = ".claude/skills/bender-*/";

function ensureGitignore(projectRoot: string): void {
  const gitignorePath = join(projectRoot, ".gitignore");
  if (existsSync(gitignorePath)) {
    const content = readFileSync(gitignorePath, "utf-8");
    if (content.includes(GITIGNORE_ENTRY)) return;
  }
  appendFileSync(gitignorePath, `\n${GITIGNORE_ENTRY}\n`, "utf-8");
  console.log(`  added ${GITIGNORE_ENTRY} to .gitignore`);
}
