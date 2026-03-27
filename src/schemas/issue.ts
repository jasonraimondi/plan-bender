import { z } from "zod";
import type { Config } from "../config/schema.js";
import type { ValidationResult } from "./validation-result.js";

const PRIORITIES = ["urgent", "high", "medium", "low"] as const;

export const IssueSchema = z
  .object({
    id: z.number().int(),
    slug: z.string().min(1, "required non-empty string"),
    name: z.string().min(1, "required non-empty string"),
    track: z.string().min(1),
    status: z.string().min(1),
    priority: z.string().min(1),
    points: z.number().int().min(1),
    labels: z.array(z.string()),
    assignee: z.string().nullable(),
    blocked_by: z.array(z.number()),
    blocking: z.array(z.number()),
    branch: z.string().nullable(),
    pr: z.string().nullable(),
    linear_id: z.string().min(1).nullable(),
    linear_url: z.string().optional(),
    created: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "must be a YYYY-MM-DD date"),
    updated: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "must be a YYYY-MM-DD date"),
    tdd: z.boolean(),
    headed: z.boolean().optional(),
    outcome: z.string().min(1, "required non-empty string"),
    scope: z.string().min(1, "required non-empty string"),
    acceptance_criteria: z.array(z.string()),
    steps: z.array(z.string()),
    use_cases: z.array(z.string()),
    notes: z.string().nullable().optional(),
  })
  .passthrough()
  .superRefine((issue, ctx) => {
    if (issue.blocked_by.includes(issue.id)) {
      ctx.addIssue({
        code: "custom",
        path: ["blocked_by"],
        message: "cannot reference self",
      });
    }
    if (issue.blocking.includes(issue.id)) {
      ctx.addIssue({
        code: "custom",
        path: ["blocking"],
        message: "cannot reference self",
      });
    }
    if (new Set(issue.blocked_by).size !== issue.blocked_by.length) {
      ctx.addIssue({
        code: "custom",
        path: ["blocked_by"],
        message: "contains duplicates",
      });
    }
    if (new Set(issue.blocking).size !== issue.blocking.length) {
      ctx.addIssue({
        code: "custom",
        path: ["blocking"],
        message: "contains duplicates",
      });
    }
  });

export type IssueYaml = z.infer<typeof IssueSchema>;

function formatPath(path: PropertyKey[]): string {
  let result = "";
  for (const segment of path) {
    if (typeof segment === "number") {
      result += `[${segment}]`;
    } else {
      const s = String(segment);
      result += result ? `.${s}` : s;
    }
  }
  return result;
}

export function validateIssue(
  data: unknown,
  file: string,
  config: Config,
): ValidationResult {
  if (!data || typeof data !== "object") {
    return { file, errors: ["Not a valid YAML object"] };
  }

  const errors: string[] = [];

  // Structural validation via Zod
  const result = IssueSchema.safeParse(data);
  if (!result.success) {
    for (const issue of result.error.issues) {
      const path = formatPath(issue.path);
      errors.push(path ? `${path}: ${issue.message}` : issue.message);
    }
  }

  const issue = data as Record<string, unknown>;

  // Config-dependent validation (tracks, workflow_states, points cap, custom fields)
  if (typeof issue.track === "string" && !config.tracks.includes(issue.track)) {
    errors.push(
      `track: must be one of ${config.tracks.join(", ")}, got "${issue.track}"`,
    );
  }

  if (
    typeof issue.status === "string" &&
    !config.workflow_states.includes(issue.status)
  ) {
    errors.push(
      `status: must be one of ${config.workflow_states.join(", ")}, got "${issue.status}"`,
    );
  }

  if (!PRIORITIES.includes(issue.priority as typeof PRIORITIES[number])) {
    errors.push(
      `priority: must be one of ${PRIORITIES.join(", ")}, got "${issue.priority}"`,
    );
  }

  if (
    typeof issue.points === "number" &&
    Number.isInteger(issue.points) &&
    issue.points > config.max_points
  ) {
    errors.push(
      `points: must be integer 1-${config.max_points}, got ${issue.points}`,
    );
  }

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
