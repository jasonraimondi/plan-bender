import { defineCommand } from "citty";
import { mkdirSync, writeFileSync, renameSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { resolveConfig } from "../../config/index.js";
import { validateIssue } from "../../schemas/issue.js";
import { readInput } from "../shared.js";

export const writeIssueCommand = defineCommand({
  meta: {
    name: "write-issue",
    description: "Validate and write an issue YAML file",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug",
      required: true,
    },
    id: {
      type: "positional",
      description: "Issue ID",
      required: true,
    },
    file: {
      type: "string",
      description: "Read YAML from file instead of stdin",
    },
  },
  async run({ args }) {
    const config = resolveConfig(process.cwd());
    const input = readInput(args);

    const data = parseYaml(input) as Record<string, unknown>;
    const result = validateIssue(data, "input", config);

    if (result.errors.length > 0) {
      console.error("Validation failed:");
      for (const err of result.errors) console.error(`  - ${err}`);
      process.exit(1);
    }

    const issueSlug = data.slug as string;
    const issuesDir = join(config.plans_dir, args.slug, "issues");
    const outPath = join(issuesDir, `${args.id}-${issueSlug}.yaml`);
    mkdirSync(issuesDir, { recursive: true });

    // Atomic write
    const tmpPath = `${outPath}.tmp`;
    writeFileSync(tmpPath, input, "utf-8");
    renameSync(tmpPath, outPath);

    console.log(`Written: ${outPath}`);
  },
});
