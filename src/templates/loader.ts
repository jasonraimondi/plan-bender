import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BUNDLED_DIR = join(__dirname, "..", "..", "templates");

export function resolveTemplates(
  projectRoot: string,
): Map<string, string> {
  const templates = new Map<string, string>();

  // Load bundled templates
  const bundledDir = existsSync(BUNDLED_DIR) ? BUNDLED_DIR : join(__dirname, "..", "templates");
  if (existsSync(bundledDir)) {
    for (const file of readdirSync(bundledDir)) {
      if (!file.endsWith(".skill.tmpl")) continue;
      const name = file.replace(".skill.tmpl", "");
      templates.set(name, readFileSync(join(bundledDir, file), "utf-8"));
    }
  }

  // Override with local templates
  const localDir = join(projectRoot, ".plan-bender", "templates");
  if (existsSync(localDir)) {
    for (const file of readdirSync(localDir)) {
      if (!file.endsWith(".skill.tmpl")) continue;
      const name = file.replace(".skill.tmpl", "");
      templates.set(name, readFileSync(join(localDir, file), "utf-8"));
    }
  }

  return templates;
}
