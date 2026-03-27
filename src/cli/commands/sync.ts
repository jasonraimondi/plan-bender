import { defineCommand } from "citty";
import {
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { parse as parseYaml, stringify as toYaml } from "yaml";
import { z } from "zod";
import { resolveConfig } from "../../config/index.js";
import { createBackend } from "../../backends/factory.js";
import type { IssueYaml } from "../../schemas/issue.js";
import type { SyncResult } from "../../backends/types.js";

const SyncArgsSchema = z.object({
  target: z.string().min(1, "target is required (slug or slug#id)"),
  pull: z.boolean(),
});

export const syncCommand = defineCommand({
  meta: {
    name: "sync",
    description: "Sync plan state with configured backend. Usage: plan-bender sync <slug> [--pull] or plan-bender sync <slug>#<id>",
  },
  args: {
    target: {
      type: "positional",
      description: "Plan slug or slug#id for per-issue sync",
      required: true,
    },
    pull: {
      type: "boolean",
      description: "Pull remote state to local YAML",
      default: false,
    },
  },
  async run({ args }) {
    const parsed = SyncArgsSchema.safeParse(args);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) console.error(issue.message);
      process.exit(1);
    }
    const config = resolveConfig(process.cwd());
    const backend = createBackend(config);

    // Parse target: "slug" or "slug#id"
    const parts = args.target.split("#");
    const slug = parts[0]!;
    let issueId: number | undefined;
    if (parts.length > 1) {
      issueId = Number(parts[1]);
      if (Number.isNaN(issueId)) {
        console.error(`Invalid issue ID: must be a number, got "${parts[1]}"`);
        process.exit(1);
      }
    }

    const planDir = join(config.plans_dir, slug);
    const issuesDir = join(planDir, "issues");

    const result: SyncResult = {
      created: 0,
      updated: 0,
      unchanged: 0,
      failed: 0,
      errors: [],
    };

    if (issueId !== undefined) {
      // Per-issue sync
      const issue = loadIssue(issuesDir, issueId);
      if (!issue) {
        console.error(`Issue #${issueId} not found in ${issuesDir}`);
        process.exit(1);
      }
      if (args.pull) {
        await pullIssue(issue, backend, issuesDir, result);
      } else {
        await pushIssue(issue, slug, backend, issuesDir, result);
      }
    } else {
      // Batch sync
      const issues = loadAllIssues(issuesDir);
      for (const issue of issues) {
        if (args.pull) {
          await pullIssue(issue, backend, issuesDir, result);
        } else {
          await pushIssue(issue, slug, backend, issuesDir, result);
        }
      }
    }

    console.log(
      `\nSync ${args.pull ? "pull" : "push"}: ${result.created} created, ${result.updated} updated, ${result.unchanged} unchanged, ${result.failed} failed`,
    );
    for (const err of result.errors) console.error(`  - ${err}`);
    if (result.failed > 0) process.exit(1);
  },
});

async function pushIssue(
  issue: IssueYaml,
  slug: string,
  backend: ReturnType<typeof createBackend>,
  issuesDir: string,
  result: SyncResult,
): Promise<void> {
  try {
    if (issue.linear_id) {
      await backend.updateIssue(issue);
      result.updated++;
    } else {
      const remote = await backend.createIssue(issue, slug);
      // Write remote ID back to local YAML
      issue.linear_id = remote.id;
      writeIssueFile(issuesDir, issue);
      result.created++;
    }
  } catch (err) {
    result.failed++;
    result.errors.push(`Issue #${issue.id}: ${err instanceof Error ? err.message : String(err)}`);
  }
}

async function pullIssue(
  issue: IssueYaml,
  backend: ReturnType<typeof createBackend>,
  issuesDir: string,
  result: SyncResult,
): Promise<void> {
  if (!issue.linear_id) {
    result.unchanged++;
    return;
  }
  try {
    const remote = await backend.pullIssue(issue.linear_id);
    const changed =
      issue.status !== remote.status ||
      issue.priority !== (remote.priority ?? issue.priority);
    if (changed) {
      issue.status = remote.status;
      if (remote.priority) issue.priority = remote.priority;
      writeIssueFile(issuesDir, issue);
      result.updated++;
    } else {
      result.unchanged++;
    }
  } catch (err) {
    result.failed++;
    result.errors.push(`Issue #${issue.id}: ${err instanceof Error ? err.message : String(err)}`);
  }
}

function loadIssue(issuesDir: string, id: number): IssueYaml | undefined {
  const files = readdirSync(issuesDir).filter((f: string) => f.startsWith(`${id}-`));
  if (files.length === 0) return undefined;
  return parseYaml(
    readFileSync(join(issuesDir, files[0]!), "utf-8"),
  ) as IssueYaml;
}

function loadAllIssues(issuesDir: string): IssueYaml[] {
  try {
    return readdirSync(issuesDir)
      .filter((f: string) => f.endsWith(".yaml"))
      .sort()
      .map((f: string) =>
        parseYaml(readFileSync(join(issuesDir, f), "utf-8")) as IssueYaml,
      );
  } catch {
    return [];
  }
}

function writeIssueFile(issuesDir: string, issue: IssueYaml): void {
  const filename = `${issue.id}-${issue.slug}.yaml`;
  writeFileSync(join(issuesDir, filename), toYaml(issue), "utf-8");
}
