export type Backend = "yaml-fs" | "linear";

export type CustomFieldType = "string" | "number" | "boolean" | "enum";

export interface CustomFieldDef {
  name: string;
  type: CustomFieldType;
  required: boolean;
  enum_values?: string[];
}

export interface LinearConfig {
  api_key?: string;
  team?: string;
  project_id?: string;
  status_map?: Record<string, string>;
}

export interface PipelineConfig {
  skip: string[];
}

export interface IssueSchemaConfig {
  custom_fields: CustomFieldDef[];
}

export interface Config {
  backend: Backend;
  tracks: string[];
  workflow_states: string[];
  step_pattern: string;
  plans_dir: string;
  max_points: number;
  pipeline: PipelineConfig;
  issue_schema: IssueSchemaConfig;
  linear: LinearConfig;
}

export type PartialConfig = {
  [K in keyof Config]?: K extends "pipeline"
    ? Partial<PipelineConfig>
    : K extends "issue_schema"
      ? Partial<IssueSchemaConfig>
      : K extends "linear"
        ? Partial<LinearConfig>
        : Config[K];
};
