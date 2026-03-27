import type { PartialConfig, Config } from "./schema.js";
import { DEFAULT_CONFIG } from "./defaults.js";

function isPlainObject(val: unknown): val is Record<string, unknown> {
  return typeof val === "object" && val !== null && !Array.isArray(val);
}

/** Deep merge with array-replace semantics. Later sources win. */
export function deepMerge(
  ...layers: (PartialConfig | undefined)[]
): Config {
  const result: Record<string, unknown> = structuredClone(
    DEFAULT_CONFIG as unknown as Record<string, unknown>,
  );
  for (const layer of layers) {
    if (!layer) continue;
    mergeInto(result, layer as unknown as Record<string, unknown>);
  }
  return result as unknown as Config;
}

function mergeInto(
  target: Record<string, unknown>,
  source: Record<string, unknown>,
): void {
  for (const key of Object.keys(source)) {
    const srcVal = source[key];
    const tgtVal = target[key];
    if (isPlainObject(srcVal) && isPlainObject(tgtVal)) {
      mergeInto(tgtVal, srcVal);
    } else {
      target[key] = structuredClone(srcVal);
    }
  }
}
