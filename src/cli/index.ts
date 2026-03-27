import { defineCommand, runMain } from "citty";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { validateCommand } from "./commands/validate.js";
import { generateCommand } from "./commands/generate.js";
import { installCommand } from "./commands/install.js";
import { writePrdCommand } from "./commands/write-prd.js";
import { writeIssueCommand } from "./commands/write-issue.js";
import { initCommand } from "./commands/init.js";
import { syncCommand } from "./commands/sync.js";
import { statusCommand } from "./commands/status.js";
import { graphCommand } from "./commands/graph.js";
import { archiveCommand } from "./commands/archive.js";

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
    "write-prd": writePrdCommand,
    "write-issue": writeIssueCommand,
    init: initCommand,
    sync: syncCommand,
    status: statusCommand,
    graph: graphCommand,
    archive: archiveCommand,
  },
});

runMain(main);
