import { defineCommand } from "citty";
import { resolveConfig } from "../../config/index.js";
import { generateSkills } from "../shared.js";

export const generateCommand = defineCommand({
  meta: {
    name: "generate-skills",
    description: "Render skill templates with project config",
  },
  async run() {
    const projectRoot = process.cwd();
    const config = resolveConfig(projectRoot);
    const count = generateSkills(config, projectRoot);
    console.log(`\n${count} skills generated in .plan-bender/skills/`);
  },
});
