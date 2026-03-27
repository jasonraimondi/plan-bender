export interface RemoteIssue {
  id: string;
  title: string;
  description?: string;
  status: string;
  priority?: string;
  labels?: string[];
  assignee?: string;
  url?: string;
}

export interface RemoteProject {
  id: string;
  name: string;
  url?: string;
}

export interface SyncResult {
  created: number;
  updated: number;
  unchanged: number;
  failed: number;
  errors: string[];
}
