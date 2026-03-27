import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { PartialConfigSchema } from "./schema.js";
import type { PartialConfig } from "./schema.js";
import { homedir } from "node:os";
import { ok, err, DomainError } from "../result.js";
import type { Result } from "../result.js";

export function readYamlSafe(
  path: string,
): Result<PartialConfig, DomainError> {
  try {
    const content = readFileSync(path, "utf-8");
    const raw = parseYaml(content);
    const result = PartialConfigSchema.safeParse(raw);
    if (!result.success) {
      return err(
        new DomainError(
          `Invalid config in ${path}: ${result.error.issues.map((i) => i.message).join(", ")}`,
        ),
      );
    }
    return ok(result.data);
  } catch (cause) {
    return err(
      new DomainError(`Failed to read config: ${path}`, { cause }),
    );
  }
}

export interface RawLayers {
  global: PartialConfig | undefined;
  project: PartialConfig | undefined;
  local: PartialConfig | undefined;
}

export function loadConfigLayers(projectRoot: string): RawLayers {
  const globalPath = join(
    homedir(),
    ".config",
    "plan-bender",
    "defaults.yaml",
  );
  const projectPath = join(projectRoot, "plan-bender.yaml");
  const localPath = join(projectRoot, "plan-bender.local.yaml");

  const unwrap = (r: Result<PartialConfig, DomainError>) =>
    r.ok ? r.data : undefined;

  return {
    global: unwrap(readYamlSafe(globalPath)),
    project: unwrap(readYamlSafe(projectPath)),
    local: unwrap(readYamlSafe(localPath)),
  };
}
