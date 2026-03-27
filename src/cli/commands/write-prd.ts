import { defineCommand } from "citty";
import { readFileSync, mkdirSync, writeFileSync, renameSync } from "node:fs";
import { join, dirname } from "node:path";
import { parse as parseYaml } from "yaml";
import { resolveConfig } from "../../config/index.js";
import { validatePrd } from "../../schemas/prd.js";

export const writePrdCommand = defineCommand({
  meta: {
    name: "write-prd",
    description: "Validate and write a PRD YAML file",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug",
      required: true,
    },
    file: {
      type: "string",
      description: "Read YAML from file instead of stdin",
    },
  },
  async run({ args }) {
    const config = resolveConfig(process.cwd());
    const input = args.file
      ? readFileSync(args.file, "utf-8")
      : readFileSync(0, "utf-8");

    const data = parseYaml(input);
    const result = validatePrd(data, "input");

    if (result.errors.length > 0) {
      console.error("Validation failed:");
      for (const err of result.errors) console.error(`  - ${err}`);
      process.exit(1);
    }

    const outDir = join(config.plans_dir, args.slug);
    const outPath = join(outDir, "prd.yaml");
    mkdirSync(outDir, { recursive: true });

    // Atomic write
    const tmpPath = `${outPath}.tmp`;
    writeFileSync(tmpPath, input, "utf-8");
    renameSync(tmpPath, outPath);

    console.log(`Written: ${outPath}`);
  },
});
