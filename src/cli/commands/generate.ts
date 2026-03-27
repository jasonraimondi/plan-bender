import { defineCommand } from "citty";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { resolveConfig } from "../../config/index.js";
import { resolveTemplates } from "../../templates/loader.js";
import { buildTemplateContext } from "../../templates/context.js";
import { render } from "../../engine/index.js";

export const generateCommand = defineCommand({
  meta: {
    name: "generate-skills",
    description: "Render skill templates with project config",
  },
  async run() {
    const projectRoot = process.cwd();
    const config = resolveConfig(projectRoot);
    const templates = resolveTemplates(projectRoot);
    const context = buildTemplateContext(config);

    const outDir = join(projectRoot, ".plan-bender", "skills");

    let count = 0;
    for (const [name, template] of templates) {
      const rendered = render(template, context);
      const skillDir = join(outDir, name);
      mkdirSync(skillDir, { recursive: true });
      writeFileSync(join(skillDir, "SKILL.md"), rendered, "utf-8");
      console.log(`  generated: ${name}`);
      count++;
    }

    console.log(`\n${count} skills generated in .plan-bender/skills/`);
  },
});
