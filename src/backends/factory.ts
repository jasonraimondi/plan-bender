import type { Config } from "../config/schema.js";
import type { TrackingBackend } from "./interface.js";

const registry = new Map<string, (config: Config) => TrackingBackend>();

export function registerBackend(
  name: string,
  factory: (config: Config) => TrackingBackend,
): void {
  registry.set(name, factory);
}

export function createBackend(config: Config): TrackingBackend {
  const factory = registry.get(config.backend);
  if (!factory) {
    const known = [...registry.keys()].join(", ") || "none";
    throw new Error(
      `Unknown backend "${config.backend}". Registered backends: ${known}`,
    );
  }
  return factory(config);
}
