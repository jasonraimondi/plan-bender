import type { IssueYaml } from "./issue.js";
import type { PrdYaml } from "./prd.js";

export function validateCrossRefs(
  prd: PrdYaml,
  issues: IssueYaml[],
): string[] {
  const errors: string[] = [];
  const issueIds = new Set(issues.map((i) => i.id));
  const useCaseIds = new Set((prd.use_cases ?? []).map((uc) => uc.id));

  for (const issue of issues) {
    // Check blocked_by references exist
    for (const dep of issue.blocked_by ?? []) {
      if (!issueIds.has(dep)) {
        errors.push(
          `Issue #${issue.id} (${issue.slug}): blocked_by references non-existent issue #${dep}`,
        );
      }
    }

    // Check blocking references exist
    for (const dep of issue.blocking ?? []) {
      if (!issueIds.has(dep)) {
        errors.push(
          `Issue #${issue.id} (${issue.slug}): blocking references non-existent issue #${dep}`,
        );
      }
    }

    // Check blocked_by/blocking symmetry
    for (const dep of issue.blocked_by ?? []) {
      const blocker = issues.find((i) => i.id === dep);
      if (blocker && !(blocker.blocking ?? []).includes(issue.id)) {
        errors.push(
          `Issue #${issue.id} blocked_by #${dep}, but #${dep} does not list #${issue.id} in blocking`,
        );
      }
    }

    // Check use_case references exist in PRD
    for (const uc of issue.use_cases ?? []) {
      if (!useCaseIds.has(uc)) {
        errors.push(
          `Issue #${issue.id} (${issue.slug}): references non-existent use case ${uc}`,
        );
      }
    }
  }

  return errors;
}
