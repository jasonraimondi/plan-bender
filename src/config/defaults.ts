import type { Config } from "./schema.js";

export const DEFAULT_CONFIG: Config = {
  backend: "yaml-fs",
  tracks: ["intent", "experience", "data", "rules", "resilience"],
  workflow_states: [
    "backlog",
    "todo",
    "in-progress",
    "blocked",
    "in-review",
    "qa",
    "done",
    "canceled",
  ],
  step_pattern: "Target — behavior",
  plans_dir: "./plans/",
  max_points: 3,
  pipeline: {
    skip: [],
  },
  issue_schema: {
    custom_fields: [],
  },
  linear: {},
};
