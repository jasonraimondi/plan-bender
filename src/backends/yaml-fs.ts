import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  readdirSync,
} from "node:fs";
import { join } from "node:path";
import { parse as parseYaml, stringify as toYaml } from "yaml";
import type { Config } from "../config/schema.js";
import type { TrackingBackend } from "./interface.js";
import type { RemoteIssue, RemoteProject } from "./types.js";
import type { PrdYaml, IssueYaml } from "../schemas/types.js";
import { registerBackend } from "./factory.js";

class YamlFsBackend implements TrackingBackend {
  constructor(private config: Config) {}

  async createProject(prd: PrdYaml): Promise<RemoteProject> {
    const dir = join(this.config.plans_dir, prd.slug);
    mkdirSync(join(dir, "issues"), { recursive: true });
    writeFileSync(join(dir, "prd.yaml"), toYaml(prd), "utf-8");
    return { id: prd.slug, name: prd.name };
  }

  async createIssue(
    issue: IssueYaml,
    projectId: string,
  ): Promise<RemoteIssue> {
    const path = this.issuePath(projectId, issue.id, issue.slug);
    mkdirSync(join(this.config.plans_dir, projectId, "issues"), {
      recursive: true,
    });
    writeFileSync(path, toYaml(issue), "utf-8");
    return this.issueToRemote(issue);
  }

  async updateIssue(issue: IssueYaml): Promise<RemoteIssue> {
    // Find the issue file by scanning the issues directory
    const projectSlug = this.findProjectForIssue(issue.id);
    if (!projectSlug) throw new Error(`Cannot find project for issue #${issue.id}`);
    const path = this.issuePath(projectSlug, issue.id, issue.slug);
    writeFileSync(path, toYaml(issue), "utf-8");
    return this.issueToRemote(issue);
  }

  async pullIssue(remoteId: string): Promise<RemoteIssue> {
    // remoteId is "{projectSlug}/{id}" for yaml-fs
    const [projectSlug, idStr] = remoteId.split("/");
    const issuesDir = join(this.config.plans_dir, projectSlug, "issues");
    const files = readdirSync(issuesDir);
    const file = files.find((f) => f.startsWith(`${idStr}-`));
    if (!file) throw new Error(`Issue not found: ${remoteId}`);
    const data = parseYaml(
      readFileSync(join(issuesDir, file), "utf-8"),
    ) as IssueYaml;
    return this.issueToRemote(data);
  }

  async pullProject(
    projectId: string,
  ): Promise<{ project: RemoteProject; issues: RemoteIssue[] }> {
    const dir = join(this.config.plans_dir, projectId);
    const prd = parseYaml(
      readFileSync(join(dir, "prd.yaml"), "utf-8"),
    ) as PrdYaml;
    const issuesDir = join(dir, "issues");
    const files = readdirSync(issuesDir).filter((f) => f.endsWith(".yaml"));
    const issues = files.map((f) => {
      const data = parseYaml(
        readFileSync(join(issuesDir, f), "utf-8"),
      ) as IssueYaml;
      return this.issueToRemote(data);
    });
    return {
      project: { id: projectId, name: prd.name },
      issues,
    };
  }

  private issuePath(projectSlug: string, id: number, slug: string): string {
    return join(
      this.config.plans_dir,
      projectSlug,
      "issues",
      `${id}-${slug}.yaml`,
    );
  }

  private issueToRemote(issue: IssueYaml): RemoteIssue {
    return {
      id: String(issue.id),
      title: issue.name,
      status: issue.status,
      priority: issue.priority,
      labels: issue.labels,
      assignee: issue.assignee ?? undefined,
    };
  }

  private findProjectForIssue(issueId: number): string | undefined {
    try {
      const plans = readdirSync(this.config.plans_dir);
      for (const plan of plans) {
        const issuesDir = join(this.config.plans_dir, plan, "issues");
        try {
          const files = readdirSync(issuesDir);
          if (files.some((f) => f.startsWith(`${issueId}-`))) return plan;
        } catch {
          // no issues dir
        }
      }
    } catch {
      // no plans dir
    }
    return undefined;
  }
}

registerBackend("yaml-fs", (config) => new YamlFsBackend(config));

export { YamlFsBackend };
