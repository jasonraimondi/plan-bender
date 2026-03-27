import type { PrdYaml } from "../schemas/prd.js";
import type { IssueYaml } from "../schemas/issue.js";
import type { RemoteIssue, RemoteProject, SyncResult } from "./types.js";

export interface TrackingBackend {
  createProject(prd: PrdYaml): Promise<RemoteProject>;
  createIssue(issue: IssueYaml, projectId: string): Promise<RemoteIssue>;
  updateIssue(issue: IssueYaml): Promise<RemoteIssue>;
  pullIssue(remoteId: string): Promise<RemoteIssue>;
  pullProject(projectId: string): Promise<{ project: RemoteProject; issues: RemoteIssue[] }>;
}
