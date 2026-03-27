import { ConfigSchema } from "./schema.js";
import type { Config } from "./schema.js";

export class ConfigValidationError extends Error {
  constructor(public readonly errors: string[]) {
    super(
      `Config validation failed:\n${errors.map((e) => `  - ${e}`).join("\n")}`,
    );
    this.name = "ConfigValidationError";
  }
}

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

export function validateConfig(config: unknown): Config {
  const result = ConfigSchema.safeParse(config);
  if (result.success) return result.data;

  const errors = result.error.issues.map(
    (issue) => `${formatPath(issue.path)}: ${issue.message}`,
  );
  throw new ConfigValidationError(errors);
}
