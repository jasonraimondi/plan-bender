import type { Config } from "../config/schema.js";

interface TrackDescription {
  name: string;
  description: string;
}

interface PipelinePhase {
  name: string;
  skill: string;
  description: string;
}

const ALL_PIPELINE_PHASES: PipelinePhase[] = [
  { name: "Interview", skill: "planning-interview-me", description: "stress-test a plan idea" },
  { name: "Write a PRD", skill: "planning-write-a-prd", description: "create a structured PRD" },
  { name: "PRD to Issues", skill: "planning-prd-to-issues", description: "break PRD into issues" },
  { name: "Write an Issue", skill: "planning-write-an-issue", description: "create a single issue" },
  { name: "Review PRD", skill: "planning-review-prd", description: "review plan as principal engineer" },
  { name: "Implement PRD", skill: "implement-prd", description: "work through issues in order" },
  { name: "Implement Issue", skill: "implement-issue", description: "implement a single issue" },
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
): Record<string, unknown> {
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
    linear: config.linear,
  };
}
