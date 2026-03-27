import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import type { Config } from "../config/schema.js";
import type { PlanValidationResult } from "./validation-result.js";
import type { IssueYaml } from "./issue.js";
import type { PrdYaml } from "./prd.js";
import { validatePrd } from "./prd.js";
import { validateIssue } from "./issue.js";
import { validateCrossRefs } from "./cross-refs.js";
import { detectCycles } from "./cycles.js";

export function validatePlan(
  slug: string,
  config: Config,
  planRoot?: string,
): PlanValidationResult {
  const planDir = planRoot ?? join(config.plans_dir, slug);
  const prdPath = join(planDir, "prd.yaml");
  const issuesDir = join(planDir, "issues");

  // Validate PRD
  const prdRaw = readYamlFile(prdPath);
  const prdResult = validatePrd(prdRaw, prdPath);
  const prd = prdRaw as PrdYaml;

  // Validate issues
  const issueFiles = listYamlFiles(issuesDir);
  const issueResults = [];
  const issues: IssueYaml[] = [];

  for (const file of issueFiles) {
    const raw = readYamlFile(file);
    issueResults.push(validateIssue(raw, file, config));
    if (raw && typeof raw === "object") {
      issues.push(raw as IssueYaml);
    }
  }

  // Cross-reference checks
  const crossRef = prd ? validateCrossRefs(prd, issues) : [];

  // Cycle detection
  const cycles = detectCycles(issues);

  const hasErrors =
    prdResult.errors.length > 0 ||
    issueResults.some((r) => r.errors.length > 0) ||
    crossRef.length > 0 ||
    cycles.length > 0;

  return {
    prd: prdResult,
    issues: issueResults,
    crossRef,
    cycles,
    valid: !hasErrors,
  };
}

function readYamlFile(path: string): unknown {
  try {
    return parseYaml(readFileSync(path, "utf-8"));
  } catch {
    return null;
  }
}

function listYamlFiles(dir: string): string[] {
  try {
    return readdirSync(dir)
      .filter((f: string) => f.endsWith(".yaml"))
      .sort()
      .map((f: string) => join(dir, f));
  } catch {
    return [];
  }
}
