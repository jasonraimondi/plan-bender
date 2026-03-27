import { LinearClient } from "@linear/sdk";
import { consola } from "consola";
import type { Config } from "../config/schema.js";
import type { TrackingBackend } from "./interface.js";
import type { RemoteIssue, RemoteProject } from "./types.js";
import type { PrdYaml } from "../schemas/prd.js";
import type { IssueYaml } from "../schemas/issue.js";
import { registerBackend } from "./registry.js";

class LinearBackend implements TrackingBackend {
  private client: LinearClient;
  private teamId: string;
  private statusMap: Record<string, string>;

  constructor(config: Config) {
    if (!config.linear.api_key) throw new Error("linear.api_key required");
    if (!config.linear.team) throw new Error("linear.team required");

    this.client = new LinearClient({ apiKey: config.linear.api_key });
    this.teamId = config.linear.team;
    this.statusMap = config.linear.status_map ?? {};
  }

  async createProject(prd: PrdYaml): Promise<RemoteProject> {
    const project = await this.client.createProject({
      name: prd.name,
      description: prd.description,
      teamIds: [this.teamId],
    });
    const created = await project.project;
    if (!created) throw new Error("Failed to create Linear project");
    return {
      id: created.id,
      name: created.name,
      url: created.url,
    };
  }

  async createIssue(
    issue: IssueYaml,
    projectId: string,
  ): Promise<RemoteIssue> {
    const stateId = await this.resolveStateId(issue.status);
    const result = await this.client.createIssue({
      teamId: this.teamId,
      title: issue.name,
      description: `${issue.outcome}\n\n## Scope\n${issue.scope}`,
      projectId,
      stateId,
      priority: this.mapPriority(issue.priority),
    });
    const created = await result.issue;
    if (!created) throw new Error("Failed to create Linear issue");
    return {
      id: created.identifier,
      title: created.title,
      status: issue.status,
      url: created.url,
    };
  }

  async updateIssue(issue: IssueYaml): Promise<RemoteIssue> {
    if (!issue.linear_id) throw new Error("Issue has no linear_id");
    const linearIssue = await this.client.issue(issue.linear_id);
    const stateId = await this.resolveStateId(issue.status);
    await linearIssue.update({
      stateId,
      priority: this.mapPriority(issue.priority),
    });
    return {
      id: issue.linear_id,
      title: issue.name,
      status: issue.status,
    };
  }

  async pullIssue(remoteId: string): Promise<RemoteIssue> {
    const issue = await this.client.issue(remoteId);
    const state = await issue.state;
    return {
      id: issue.identifier,
      title: issue.title,
      status: this.reverseMapState(state?.name),
      priority: this.reversePriority(issue.priority),
      url: issue.url,
    };
  }

  async pullProject(
    projectId: string,
  ): Promise<{ project: RemoteProject; issues: RemoteIssue[] }> {
    const project = await this.client.project(projectId);
    const issues: RemoteIssue[] = [];

    let issuesConn = await project.issues();
    while (true) {
      for (const issue of issuesConn.nodes) {
        const state = await issue.state;
        issues.push({
          id: issue.identifier,
          title: issue.title,
          status: this.reverseMapState(state?.name),
          priority: this.reversePriority(issue.priority),
          url: issue.url,
        });
      }
      if (!issuesConn.pageInfo.hasNextPage) break;
      issuesConn = await issuesConn.fetchNext();
    }

    return {
      project: { id: project.id, name: project.name, url: project.url },
      issues,
    };
  }

  private async resolveStateId(localStatus: string): Promise<string | undefined> {
    const linearName = this.statusMap[localStatus] ?? localStatus;
    const states = await this.client.workflowStates({
      filter: { team: { id: { eq: this.teamId } } },
    });
    const match = states.nodes.find(
      (s) => s.name.toLowerCase() === linearName.toLowerCase(),
    );
    return match?.id;
  }

  private reverseMapState(linearName: string | undefined | null): string {
    if (!linearName) {
      consola.warn("Linear issue has no workflow state — defaulting to backlog");
      return "backlog";
    }
    const matches = Object.entries(this.statusMap).filter(
      ([, remote]) => remote.toLowerCase() === linearName.toLowerCase(),
    );
    if (matches.length > 1) {
      consola.warn(
        `Ambiguous state mapping: Linear "${linearName}" matches ${matches.map(([l]) => `"${l}"`).join(", ")} — using first`,
      );
    }
    if (matches.length > 0) return matches[0]![0];
    return linearName.toLowerCase().replace(/\s+/g, "-");
  }

  private mapPriority(priority: string): number {
    switch (priority) {
      case "urgent": return 1;
      case "high": return 2;
      case "medium": return 3;
      case "low": return 4;
      default: return 0;
    }
  }

  private reversePriority(priority: number): string {
    switch (priority) {
      case 1: return "urgent";
      case 2: return "high";
      case 3: return "medium";
      case 4: return "low";
      default: return "medium";
    }
  }
}

registerBackend("linear", (config) => new LinearBackend(config));

export { LinearBackend };
