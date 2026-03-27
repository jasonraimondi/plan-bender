export interface PrdYaml {
  name: string;
  slug: string;
  status: string;
  created: string;
  updated: string;
  description: string;
  why: string;
  outcome: string;
  in_scope?: string[];
  out_of_scope?: string[];
  use_cases?: { id: string; description: string }[];
  decisions?: string[];
  open_questions?: string[];
  risks?: string[];
  validation?: string[];
  notes?: string;
  dev_command?: string | null;
  base_url?: string | null;
  linear?: { project_id?: string };
  [key: string]: unknown;
}

export interface IssueYaml {
  id: number;
  slug: string;
  name: string;
  track: string;
  status: string;
  priority: string;
  points: number;
  labels: string[];
  assignee: string | null;
  blocked_by: number[];
  blocking: number[];
  branch: string | null;
  pr: string | null;
  linear_id: string | null;
  created: string;
  updated: string;
  tdd: boolean;
  headed?: boolean;
  outcome: string;
  scope: string;
  acceptance_criteria: string[];
  steps: string[];
  use_cases: string[];
  notes?: string;
  [key: string]: unknown;
}

export interface ValidationResult {
  file: string;
  errors: string[];
}

export interface PlanValidationResult {
  prd: ValidationResult;
  issues: ValidationResult[];
  crossRef: string[];
  cycles: string[];
  valid: boolean;
}
