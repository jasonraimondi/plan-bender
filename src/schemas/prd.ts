import type { PrdYaml, ValidationResult } from "./types.js";

const PRD_STATUSES = ["draft", "active", "in-review", "approved", "complete", "archived"];

const REQUIRED_STRINGS: (keyof PrdYaml)[] = [
  "name",
  "slug",
  "status",
  "description",
  "why",
  "outcome",
];

export function validatePrd(data: unknown, file: string): ValidationResult {
  const errors: string[] = [];
  if (!data || typeof data !== "object") {
    return { file, errors: ["Not a valid YAML object"] };
  }

  const prd = data as Record<string, unknown>;

  for (const field of REQUIRED_STRINGS) {
    if (typeof prd[field] !== "string" || !(prd[field] as string).trim()) {
      errors.push(`${field}: required non-empty string`);
    }
  }

  if (prd.status && !PRD_STATUSES.includes(prd.status as string)) {
    errors.push(
      `status: must be one of ${PRD_STATUSES.join(", ")}, got "${prd.status}"`,
    );
  }

  return { file, errors };
}
