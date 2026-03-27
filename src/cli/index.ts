import { defineCommand, runMain } from "citty";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { validateCommand } from "./commands/validate.js";
import { generateCommand } from "./commands/generate.js";
import { installCommand } from "./commands/install.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

function getVersion(): string {
  try {
    const pkg = JSON.parse(
      readFileSync(join(__dirname, "..", "package.json"), "utf-8"),
    );
    return pkg.version;
  } catch {
    return "0.0.0";
  }
}

const main = defineCommand({
  meta: {
    name: "plan-bender",
    version: getVersion(),
    description:
      "A framework + CLI for configurable, template-driven planning with pluggable tracking backends.",
  },
  subCommands: {
    validate: validateCommand,
    "generate-skills": generateCommand,
    install: installCommand,
  },
});

runMain(main);
