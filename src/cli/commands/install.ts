import { defineCommand } from "citty";
import {
  readdirSync,
  existsSync,
  symlinkSync,
  unlinkSync,
  lstatSync,
  mkdirSync,
} from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";

export const installCommand = defineCommand({
  meta: {
    name: "install",
    description: "Symlink generated skills to ~/.claude/skills/",
  },
  async run() {
    const projectRoot = process.cwd();
    const sourceDir = join(projectRoot, ".plan-bender", "skills");
    const targetDir = join(homedir(), ".claude", "skills");

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

    console.log(`\n${count} skills installed to ${targetDir}`);
  },
});
