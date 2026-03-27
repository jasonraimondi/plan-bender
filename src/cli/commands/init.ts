import { defineCommand } from "citty";
import { existsSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { stringify as toYaml } from "yaml";
import { consola } from "consola";
import { resolveConfig } from "../../config/index.js";
import { generateSkills } from "../shared.js";
import type { PartialConfig } from "../../config/schema.js";

export const initCommand = defineCommand({
  meta: {
    name: "init",
    description: "Initialize plan-bender in the current project",
  },
  args: {
    force: {
      type: "boolean",
      description: "Overwrite existing config",
      default: false,
    },
  },
  async run({ args }) {
    const projectRoot = process.cwd();
    const configPath = join(projectRoot, "plan-bender.yaml");

    if (existsSync(configPath) && !args.force) {
      const overwrite = await consola.prompt(
        "plan-bender.yaml already exists. Overwrite?",
        { type: "confirm" },
      );
      if (!overwrite) {
        console.log("Aborted.");
        return;
      }
    }

    // Interactive prompts
    const backend = await consola.prompt("Backend:", {
      type: "select",
      options: ["yaml-fs", "linear"],
      initial: "yaml-fs",
    });

    const tracksChoice = await consola.prompt("Tracks:", {
      type: "select",
      options: [
        "default (intent, experience, data, rules, resilience)",
        "custom",
      ],
      initial: "default (intent, experience, data, rules, resilience)",
    });

    let tracks: string[] | undefined;
    if (tracksChoice === "custom") {
      const custom = await consola.prompt(
        "Enter tracks (comma-separated):",
        { type: "text" },
      );
      tracks = (custom as string).split(",").map((t) => t.trim()).filter(Boolean);
    }

    const plansDir = await consola.prompt("Plans directory:", {
      type: "text",
      default: "./plans/",
      initial: "./plans/",
    });

    const maxPoints = await consola.prompt("Max points per issue:", {
      type: "text",
      default: "3",
      initial: "3",
    });

    // Build config
    const config: PartialConfig = {
      backend: backend as "yaml-fs" | "linear",
    };
    if (tracks) config.tracks = tracks;
    if (plansDir !== "./plans/") config.plans_dir = plansDir as string;
    if (maxPoints !== "3") config.max_points = parseInt(maxPoints as string, 10);

    if (backend === "linear") {
      const apiKey = await consola.prompt("Linear API key:", {
        type: "text",
      });
      const team = await consola.prompt("Linear team ID:", {
        type: "text",
      });
      config.linear = {
        api_key: apiKey as string,
        team: team as string,
      };
    }

    // Write config
    const yamlContent = toYaml(config, { lineWidth: 120 });
    writeFileSync(configPath, yamlContent, "utf-8");
    console.log(`\nWritten: ${configPath}`);

    // Generate skills
    const resolved = resolveConfig(projectRoot);
    const count = generateSkills(resolved, projectRoot);
    console.log(`Generated ${count} skills in .plan-bender/skills/`);

    console.log("\nDone! Next steps:");
    console.log("  1. Run `plan-bender install` to symlink skills to ~/.claude/skills/");
    console.log("  2. Try `/plan-bender` in Claude Code to get started");
  },
});
