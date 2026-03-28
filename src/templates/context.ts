import type { Config, CustomFieldDef, PipelineConfig } from "../config/schema.js";

interface TrackDescription {
  name: string;
  description: string;
}

interface PipelinePhase {
  name: string;
  skill: string;
  description: string;
}

export interface TemplateContext extends Record<string, unknown> {
  plans_dir: string;
  tracks: string[];
  workflow_states: string[];
  step_pattern: string;
  max_points: number;
  backend: string;
  has_backend_sync: boolean;
  custom_fields: CustomFieldDef[];
  track_descriptions: TrackDescription[];
  pipeline: PipelineConfig;
  pipeline_phases: PipelinePhase[];
}

const ALL_PIPELINE_PHASES: PipelinePhase[] = [
  { name: "Interview", skill: "bender-interview-me", description: "stress-test a plan idea" },
  { name: "Write a PRD", skill: "bender-write-a-prd", description: "create a structured PRD" },
  { name: "PRD to Issues", skill: "bender-prd-to-issues", description: "break PRD into issues" },
  { name: "Write an Issue", skill: "bender-write-an-issue", description: "create a single issue" },
  { name: "Review PRD", skill: "bender-review-prd", description: "review plan as principal engineer" },
  { name: "Implement PRD", skill: "bender-implement-prd", description: "work through issues in order" },
  { name: "Implement Issue", skill: "bender-implement-issue", description: "implement a single issue" },
];

const DEFAULT_TRACK_DESCRIPTIONS: Record<string, string> = {
  intent: "API endpoints, service wiring, core business flow",
  experience:
    "UI components, navigation, visual feedback, user-facing interactions",
  data: "Schema, migrations, CRUD operations, entity lifecycle, queries",
  rules: "Authorization, validation, business rules, permission boundaries",
  resilience:
    "Error handling, retry logic, failure modes, rate limiting, external dependency fallbacks",
};

export function buildTemplateContext(
  config: Config,
): TemplateContext {
  const trackDescriptions: TrackDescription[] = config.tracks.map((t) => ({
    name: t,
    description: DEFAULT_TRACK_DESCRIPTIONS[t] ?? `${t} track`,
  }));

  return {
    plans_dir: config.plans_dir,
    tracks: config.tracks,
    workflow_states: config.workflow_states,
    step_pattern: config.step_pattern,
    max_points: config.max_points,
    backend: config.backend,
    has_backend_sync: config.backend !== "yaml-fs",
    custom_fields: config.issue_schema.custom_fields,
    track_descriptions: trackDescriptions,
    pipeline: config.pipeline,
    pipeline_phases: ALL_PIPELINE_PHASES.filter(
      (p) => !config.pipeline.skip.includes(p.skill),
    ),
  };
}
