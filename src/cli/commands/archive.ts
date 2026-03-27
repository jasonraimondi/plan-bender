import { defineCommand } from "citty";
import {
  readFileSync,
  readdirSync,
  existsSync,
  mkdirSync,
  renameSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { consola } from "consola";
import { resolveConfig } from "../../config/index.js";
import type { IssueYaml } from "../../schemas/issue.js";

export const archiveCommand = defineCommand({
  meta: {
    name: "archive",
    description: "Archive a completed plan",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug to archive",
      required: true,
    },
    force: {
      type: "boolean",
      description: "Skip confirmation and active-issue check",
      default: false,
    },
  },
  async run({ args }) {
    const config = resolveConfig(process.cwd());
    const planDir = join(config.plans_dir, args.slug);

    if (!existsSync(planDir)) {
      console.error(`Plan not found: ${planDir}`);
      process.exit(1);
    }

    const issues = loadIssues(join(planDir, "issues"));

    // Check for active issues
    const active = issues.filter((i) =>
      ["in-progress", "blocked"].includes(i.status),
    );
    if (active.length > 0 && !args.force) {
      console.error(
        `Cannot archive: ${active.length} active issues (${active.map((i) => `#${i.id}`).join(", ")})`,
      );
      console.error("Use --force to override.");
      process.exit(1);
    }

    // Confirm
    if (!args.force) {
      const confirm = await consola.prompt(
        `Archive plan "${args.slug}"?`,
        { type: "confirm" },
      );
      if (!confirm) {
        console.log("Aborted.");
        return;
      }
    }

    // Generate summary
    const doneCount = issues.filter((i) => i.status === "done").length;
    const totalPoints = issues.reduce((s, i) => s + i.points, 0);
    const donePoints = issues
      .filter((i) => i.status === "done")
      .reduce((s, i) => s + i.points, 0);

    const byStatus: Record<string, number> = {};
    for (const issue of issues) {
      byStatus[issue.status] = (byStatus[issue.status] ?? 0) + 1;
    }

    const summary = [
      `# Archive Summary: ${args.slug}`,
      "",
      `Archived: ${new Date().toISOString().slice(0, 10)}`,
      "",
      `## Stats`,
      `- Issues: ${doneCount}/${issues.length} done`,
      `- Points: ${donePoints}/${totalPoints}`,
      "",
      "## By Status",
      ...Object.entries(byStatus).map(([s, c]) => `- ${s}: ${c}`),
    ].join("\n");

    // Move to archive
    const archiveDir = join(config.plans_dir, ".archive");
    const archiveDest = join(archiveDir, args.slug);
    mkdirSync(archiveDir, { recursive: true });
    renameSync(planDir, archiveDest);

    // Write summary
    writeFileSync(join(archiveDest, "summary.md"), summary, "utf-8");

    console.log(`Archived: ${archiveDest}`);
  },
});

function loadIssues(dir: string): IssueYaml[] {
  try {
    return readdirSync(dir)
      .filter((f: string) => f.endsWith(".yaml"))
      .map((f: string) =>
        parseYaml(readFileSync(join(dir, f), "utf-8")) as IssueYaml,
      );
  } catch {
    return [];
  }
}
