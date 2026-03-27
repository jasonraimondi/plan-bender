import { describe, it, expect } from "vitest";
import { execFileSync } from "node:child_process";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const cli = join(__dirname, "..", "..", "dist", "cli.js");

describe("plan-bender CLI", () => {
  it("prints version", () => {
    const out = execFileSync("node", [cli, "--version"], {
      encoding: "utf-8",
    }).trim();
    expect(out).toMatch(/^\d+\.\d+\.\d+$/);
  });
});
