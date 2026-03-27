import type { Config, Backend, CustomFieldType } from "./schema.js";

export class ConfigValidationError extends Error {
  constructor(public readonly errors: string[]) {
    super(`Config validation failed:\n${errors.map((e) => `  - ${e}`).join("\n")}`);
    this.name = "ConfigValidationError";
  }
}

const VALID_BACKENDS: Backend[] = ["yaml-fs", "linear"];
const VALID_FIELD_TYPES: CustomFieldType[] = [
  "string",
  "number",
  "boolean",
  "enum",
];

export function validateConfig(config: Config): Config {
  const errors: string[] = [];

  if (!VALID_BACKENDS.includes(config.backend)) {
    errors.push(
      `backend: must be one of ${VALID_BACKENDS.join(", ")}, got "${config.backend}"`,
    );
  }

  if (!Array.isArray(config.tracks) || config.tracks.length === 0) {
    errors.push("tracks: must be a non-empty array of strings");
  }

  if (
    !Array.isArray(config.workflow_states) ||
    config.workflow_states.length === 0
  ) {
    errors.push("workflow_states: must be a non-empty array of strings");
  }

  if (typeof config.step_pattern !== "string" || !config.step_pattern) {
    errors.push("step_pattern: must be a non-empty string");
  }

  if (typeof config.plans_dir !== "string" || !config.plans_dir) {
    errors.push("plans_dir: must be a non-empty string");
  }

  if (
    typeof config.max_points !== "number" ||
    config.max_points < 1 ||
    !Number.isInteger(config.max_points)
  ) {
    errors.push("max_points: must be a positive integer");
  }

  if (!Array.isArray(config.pipeline?.skip)) {
    errors.push("pipeline.skip: must be an array");
  }

  for (const [i, field] of (config.issue_schema?.custom_fields ?? []).entries()) {
    const prefix = `issue_schema.custom_fields[${i}]`;
    if (!field.name) {
      errors.push(`${prefix}.name: required`);
    }
    if (!VALID_FIELD_TYPES.includes(field.type)) {
      errors.push(
        `${prefix}.type: must be one of ${VALID_FIELD_TYPES.join(", ")}`,
      );
    }
    if (field.type === "enum" && (!field.enum_values || field.enum_values.length === 0)) {
      errors.push(`${prefix}.enum_values: required when type is "enum"`);
    }
  }

  if (config.backend === "linear") {
    if (!config.linear?.api_key) {
      errors.push("linear.api_key: required when backend is linear");
    }
    if (!config.linear?.team) {
      errors.push("linear.team: required when backend is linear");
    }
  }

  if (errors.length > 0) {
    throw new ConfigValidationError(errors);
  }

  return config;
}
