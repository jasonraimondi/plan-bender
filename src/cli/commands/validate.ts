import { defineCommand } from "citty";
import { z } from "zod";
import { resolveConfig } from "../../config/index.js";
import { validatePlan } from "../../schemas/validate.js";

const ValidateArgsSchema = z.object({
  slug: z.string().min(1, "slug is required"),
});

export const validateCommand = defineCommand({
  meta: {
    name: "validate",
    description: "Validate plan YAML files against schemas. Usage: plan-bender validate <slug>",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug to validate",
      required: true,
    },
  },
  async run({ args }) {
    const parsed = ValidateArgsSchema.safeParse(args);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) console.error(issue.message);
      process.exit(1);
    }
    const config = resolveConfig(process.cwd());
    const result = validatePlan(args.slug, config);

    let hasOutput = false;

    if (result.prd.errors.length > 0) {
      console.error(`PRD (${result.prd.file}):`);
      for (const err of result.prd.errors)
        console.error(`  - ${err}`);
      hasOutput = true;
    }

    for (const issue of result.issues) {
      if (issue.errors.length > 0) {
        console.error(`Issue (${issue.file}):`);
        for (const err of issue.errors)
          console.error(`  - ${err}`);
        hasOutput = true;
      }
    }

    if (result.crossRef.length > 0) {
      console.error("Cross-reference errors:");
      for (const err of result.crossRef)
        console.error(`  - ${err}`);
      hasOutput = true;
    }

    if (result.cycles.length > 0) {
      console.error("Dependency cycles:");
      for (const err of result.cycles)
        console.error(`  - ${err}`);
      hasOutput = true;
    }

    if (result.valid) {
      console.log(`Plan "${args.slug}" is valid.`);
    } else {
      if (!hasOutput) console.error("Validation failed.");
      process.exit(1);
    }
  },
});
