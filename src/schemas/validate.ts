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
import { ok, err, DomainError } from "../result.js";
import type { Result } from "../result.js";

export function validatePlan(
  slug: string,
  config: Config,
  planRoot?: string,
): PlanValidationResult {
  const planDir = planRoot ?? join(config.plans_dir, slug);
  const prdPath = join(planDir, "prd.yaml");
  const issuesDir = join(planDir, "issues");

  // Validate PRD
  const prdRead = readYamlFile(prdPath);
  const prdRaw = prdRead.ok ? prdRead.data : null;
  const prdResult = prdRead.ok
    ? validatePrd(prdRaw, prdPath)
    : { file: prdPath, errors: [prdRead.error.message] };
  const prd = prdRaw as PrdYaml;

  // Validate issues
  const filesRead = listYamlFiles(issuesDir);
  const issueFiles = filesRead.ok ? filesRead.data : [];
  const issueResults = [];
  const issues: IssueYaml[] = [];

  if (!filesRead.ok) {
    issueResults.push({
      file: issuesDir,
      errors: [filesRead.error.message],
    });
  }

  for (const file of issueFiles) {
    const rawRead = readYamlFile(file);
    if (!rawRead.ok) {
      issueResults.push({ file, errors: [rawRead.error.message] });
      continue;
    }
    const raw = rawRead.data;
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

export function readYamlFile(
  path: string,
): Result<unknown, DomainError> {
  try {
    return ok(parseYaml(readFileSync(path, "utf-8")));
  } catch (cause) {
    return err(
      new DomainError(`Failed to read YAML file: ${path}`, { cause }),
    );
  }
}

export function listYamlFiles(
  dir: string,
): Result<string[], DomainError> {
  try {
    const files = readdirSync(dir)
      .filter((f: string) => f.endsWith(".yaml"))
      .sort()
      .map((f: string) => join(dir, f));
    return ok(files);
  } catch (cause) {
    return err(
      new DomainError(`Failed to list YAML files in: ${dir}`, { cause }),
    );
  }
}
