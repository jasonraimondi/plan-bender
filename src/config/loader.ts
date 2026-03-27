import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import type { PartialConfig } from "./schema.js";
import { homedir } from "node:os";

function readYamlSafe(path: string): PartialConfig | undefined {
  try {
    const content = readFileSync(path, "utf-8");
    return parseYaml(content) as PartialConfig;
  } catch {
    return undefined;
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

  return {
    global: readYamlSafe(globalPath),
    project: readYamlSafe(projectPath),
    local: readYamlSafe(localPath),
  };
}
