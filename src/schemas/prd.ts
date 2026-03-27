import { z } from "zod";
import type { ValidationResult } from "./validation-result.js";

const PRD_STATUSES = [
  "draft",
  "active",
  "in-review",
  "approved",
  "complete",
  "archived",
] as const;

export const PrdSchema = z
  .object({
    name: z.string().min(1, "required non-empty string"),
    slug: z.string().min(1, "required non-empty string"),
    status: z.enum(PRD_STATUSES),
    created: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "must be a YYYY-MM-DD date"),
    updated: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "must be a YYYY-MM-DD date"),
    description: z.string().min(1, "required non-empty string"),
    why: z.string().min(1, "required non-empty string"),
    outcome: z.string().min(1, "required non-empty string"),
    in_scope: z.array(z.string()).optional(),
    out_of_scope: z.array(z.string()).optional(),
    use_cases: z
      .array(z.object({ id: z.string(), description: z.string() }))
      .optional(),
    decisions: z.array(z.string()).optional(),
    open_questions: z.array(z.string()).optional(),
    risks: z.array(z.string()).optional(),
    validation: z.array(z.string()).optional(),
    notes: z.string().nullable().optional(),
    dev_command: z.string().nullable().optional(),
    base_url: z.string().nullable().optional(),
    linear: z
      .object({ project_id: z.string().optional() })
      .passthrough()
      .optional(),
  })
  .passthrough();

export type PrdYaml = z.infer<typeof PrdSchema>;

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

export function validatePrd(data: unknown, file: string): ValidationResult {
  if (!data || typeof data !== "object") {
    return { file, errors: ["Not a valid YAML object"] };
  }

  const result = PrdSchema.safeParse(data);
  if (result.success) return { file, errors: [] };

  const errors = result.error.issues.map((issue) => {
    const path = formatPath(issue.path);
    return path ? `${path}: ${issue.message}` : issue.message;
  });
  return { file, errors };
}
