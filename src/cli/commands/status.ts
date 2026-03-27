import { defineCommand } from "citty";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { resolveConfig } from "../../config/index.js";
import type { IssueYaml, PrdYaml } from "../../schemas/types.js";

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show plan status dashboard",
  },
  args: {
    slug: {
      type: "positional",
      description: "Plan slug (optional, shows all if omitted)",
      required: false,
    },
  },
  async run({ args }) {
    const config = resolveConfig(process.cwd());
    const plansDir = config.plans_dir;

    if (!existsSync(plansDir)) {
      console.log("No plans directory found.");
      return;
    }

    const slugs = args.slug
      ? [args.slug]
      : readdirSync(plansDir).filter((d: string) => {
          return existsSync(join(plansDir, d, "prd.yaml"));
        });

    if (slugs.length === 0) {
      console.log("No plans found.");
      return;
    }

    let totalDone = 0;
    let totalPoints = 0;
    let totalDonePoints = 0;

    for (const slug of slugs) {
      const planDir = join(plansDir, slug);
      const prd = readYaml<PrdYaml>(join(planDir, "prd.yaml"));
      const issues = loadIssues(join(planDir, "issues"));

      const byStatus = groupBy(issues, (i) => i.status);
      const byTrack = groupBy(issues, (i) => i.track);
      const done = (byStatus.done ?? []).length;
      const donePoints = (byStatus.done ?? []).reduce((s, i) => s + i.points, 0);
      const points = issues.reduce((s, i) => s + i.points, 0);
      const blocked = byStatus.blocked ?? [];

      totalDone += done;
      totalPoints += points;
      totalDonePoints += donePoints;

      console.log(`\n${prd?.name ?? slug} (${prd?.status ?? "unknown"})`);
      console.log(`  Issues: ${done}/${issues.length} done`);
      console.log(`  Points: ${donePoints}/${points}`);

      if (blocked.length > 0) {
        console.log(
          `  Blocked: ${blocked.map((i) => `#${i.id}`).join(", ")}`,
        );
      }

      if (args.slug) {
        // Detailed per-issue view
        console.log("\n  Issues:");
        for (const issue of issues) {
          const deps =
            issue.blocked_by.length > 0
              ? ` (blocked by ${issue.blocked_by.map((d) => `#${d}`).join(", ")})`
              : "";
          console.log(
            `    #${issue.id} [${issue.status}] ${issue.name} (${issue.track}, ${issue.points}pt)${deps}`,
          );
        }
      }

      console.log("\n  Tracks:");
      for (const track of config.tracks) {
        const trackIssues = byTrack[track] ?? [];
        const trackDone = trackIssues.filter(
          (i) => i.status === "done",
        ).length;
        console.log(`    ${track}: ${trackDone}/${trackIssues.length}`);
      }
    }

    if (slugs.length > 1) {
      console.log(`\nTotal: ${totalDone} done, ${totalDonePoints}/${totalPoints} points`);
    }
  },
});

function readYaml<T>(path: string): T | null {
  try {
    return parseYaml(readFileSync(path, "utf-8")) as T;
  } catch {
    return null;
  }
}

function loadIssues(dir: string): IssueYaml[] {
  try {
    return readdirSync(dir)
      .filter((f: string) => f.endsWith(".yaml"))
      .sort()
      .map((f: string) =>
        parseYaml(readFileSync(join(dir, f), "utf-8")) as IssueYaml,
      );
  } catch {
    return [];
  }
}

function groupBy<T>(arr: T[], key: (item: T) => string): Record<string, T[]> {
  const result: Record<string, T[]> = {};
  for (const item of arr) {
    const k = key(item);
    (result[k] ??= []).push(item);
  }
  return result;
}
