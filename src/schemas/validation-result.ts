export interface ValidationResult {
  file: string;
  errors: string[];
}

export interface PlanValidationResult {
  prd: ValidationResult;
  issues: ValidationResult[];
  crossRef: string[];
  cycles: string[];
  valid: boolean;
}
