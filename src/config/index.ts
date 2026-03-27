import type { Config } from "./schema.js";
import { loadConfigLayers } from "./loader.js";
import { deepMerge } from "./merge.js";
import { validateConfig } from "./validate.js";

export { type Config, type PartialConfig } from "./schema.js";
export { DEFAULT_CONFIG } from "./defaults.js";
export { deepMerge } from "./merge.js";
export { validateConfig, ConfigValidationError } from "./validate.js";
export { loadConfigLayers } from "./loader.js";

export function resolveConfig(projectRoot: string): Config {
  const layers = loadConfigLayers(projectRoot);
  const merged = deepMerge(layers.global, layers.project, layers.local);
  return validateConfig(merged);
}
