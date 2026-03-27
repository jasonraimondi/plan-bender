import type { Config } from "../config/schema.js";
import type { ValidationResult } from "./types.js";

const PRIORITIES = ["high", "medium", "low"];

export function validateIssue(
  data: unknown,
  file: string,
  config: Config,
): ValidationResult {
  const errors: string[] = [];
  if (!data || typeof data !== "object") {
    return { file, errors: ["Not a valid YAML object"] };
  }

  const issue = data as Record<string, unknown>;

  // Required fields
  if (typeof issue.id !== "number") errors.push("id: required number");
  if (typeof issue.slug !== "string" || !issue.slug)
    errors.push("slug: required non-empty string");
  if (typeof issue.name !== "string" || !issue.name)
    errors.push("name: required non-empty string");
  if (typeof issue.outcome !== "string" || !issue.outcome)
    errors.push("outcome: required non-empty string");
  if (typeof issue.scope !== "string" || !issue.scope)
    errors.push("scope: required non-empty string");

  // Enum fields
  if (!config.tracks.includes(issue.track as string)) {
    errors.push(
      `track: must be one of ${config.tracks.join(", ")}, got "${issue.track}"`,
    );
  }

  if (!config.workflow_states.includes(issue.status as string)) {
    errors.push(
      `status: must be one of ${config.workflow_states.join(", ")}, got "${issue.status}"`,
    );
  }

  if (!PRIORITIES.includes(issue.priority as string)) {
    errors.push(
      `priority: must be one of ${PRIORITIES.join(", ")}, got "${issue.priority}"`,
    );
  }

  // Points
  const points = issue.points;
  if (
    typeof points !== "number" ||
    !Number.isInteger(points) ||
    points < 1 ||
    points > config.max_points
  ) {
    errors.push(`points: must be integer 1-${config.max_points}, got ${points}`);
  }

  // Arrays
  if (!Array.isArray(issue.blocked_by))
    errors.push("blocked_by: must be an array");
  if (!Array.isArray(issue.blocking))
    errors.push("blocking: must be an array");
  if (!Array.isArray(issue.acceptance_criteria))
    errors.push("acceptance_criteria: must be an array");
  if (!Array.isArray(issue.steps)) errors.push("steps: must be an array");

  // Custom fields
  for (const field of config.issue_schema.custom_fields) {
    const val = issue[field.name];
    if (field.required && (val === undefined || val === null || val === "")) {
      errors.push(`${field.name}: required custom field`);
      continue;
    }
    if (val === undefined || val === null) continue;

    switch (field.type) {
      case "string":
        if (typeof val !== "string")
          errors.push(`${field.name}: must be a string`);
        break;
      case "number":
        if (typeof val !== "number")
          errors.push(`${field.name}: must be a number`);
        break;
      case "boolean":
        if (typeof val !== "boolean")
          errors.push(`${field.name}: must be a boolean`);
        break;
      case "enum":
        if (!field.enum_values?.includes(val as string))
          errors.push(
            `${field.name}: must be one of ${field.enum_values?.join(", ")}, got "${val}"`,
          );
        break;
    }
  }

  return { file, errors };
}
