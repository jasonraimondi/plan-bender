import { readFileSync, readdirSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import type { IssueYaml } from "../schemas/issue.js";
import type { Config } from "../config/schema.js";
import { resolveTemplates } from "../templates/loader.js";
import { buildTemplateContext } from "../templates/context.js";
import { render } from "../engine/index.js";

export function loadIssues(dir: string): IssueYaml[] {
  try {
    return readdirSync(dir)
      .filter((f: string) => f.endsWith(".yaml"))
      .sort()
      .map(
        (f: string) =>
          parseYaml(readFileSync(join(dir, f), "utf-8")) as IssueYaml,
      );
  } catch {
    return [];
  }
}

export function readInput(args: { file?: string }): string {
  return args.file
    ? readFileSync(args.file, "utf-8")
    : readFileSync(0, "utf-8");
}

export function generateSkills(
  config: Config,
  projectRoot: string,
): number {
  const templates = resolveTemplates(projectRoot);
  const context = buildTemplateContext(config);
  const outDir = join(projectRoot, ".plan-bender", "skills");

  let count = 0;
  for (const [name, template] of templates) {
    const rendered = render(template, context);
    const skillDir = join(outDir, name);
    mkdirSync(skillDir, { recursive: true });
    writeFileSync(join(skillDir, "SKILL.md"), rendered, "utf-8");
    count++;
  }
  return count;
}
