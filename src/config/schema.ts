import { z } from "zod";

export const BackendSchema = z.enum(["yaml-fs", "linear"]);

export const CustomFieldDefSchema = z
  .object({
    name: z.string().min(1, "required"),
    type: z.enum(["string", "number", "boolean", "enum"]),
    required: z.boolean(),
    enum_values: z.array(z.string()).optional(),
  })
  .refine(
    (f) => f.type !== "enum" || (f.enum_values && f.enum_values.length > 0),
    { message: 'required when type is "enum"', path: ["enum_values"] },
  );

export const LinearConfigSchema = z.object({
  api_key: z.string().optional(),
  team: z.string().optional(),
  project_id: z.string().optional(),
  status_map: z.record(z.string(), z.string()).optional(),
});

export const PipelineConfigSchema = z.object({
  skip: z.array(z.string()),
});

export const IssueSchemaConfigSchema = z.object({
  custom_fields: z.array(CustomFieldDefSchema),
});

export const ConfigSchema = z
  .object({
    backend: BackendSchema,
    tracks: z.array(z.string()).min(1, "must be a non-empty array of strings"),
    workflow_states: z
      .array(z.string())
      .min(1, "must be a non-empty array of strings"),
    step_pattern: z.string().min(1, "must be a non-empty string"),
    plans_dir: z.string().min(1, "must be a non-empty string"),
    max_points: z.number().int().min(1, "must be a positive integer"),
    pipeline: PipelineConfigSchema,
    issue_schema: IssueSchemaConfigSchema,
    linear: LinearConfigSchema,
  })
  .superRefine((config, ctx) => {
    if (config.backend === "linear") {
      if (!config.linear.api_key) {
        ctx.addIssue({
          code: "custom",
          path: ["linear", "api_key"],
          message: "required when backend is linear",
        });
      }
      if (!config.linear.team) {
        ctx.addIssue({
          code: "custom",
          path: ["linear", "team"],
          message: "required when backend is linear",
        });
      }
    }
  });

export const PartialConfigSchema = z
  .object({
    backend: BackendSchema.optional(),
    tracks: z.array(z.string()).optional(),
    workflow_states: z.array(z.string()).optional(),
    step_pattern: z.string().optional(),
    plans_dir: z.string().optional(),
    max_points: z.number().optional(),
    pipeline: PipelineConfigSchema.partial().optional(),
    issue_schema: IssueSchemaConfigSchema.partial().optional(),
    linear: LinearConfigSchema.partial().optional(),
  })
  .passthrough();

export type Backend = z.infer<typeof BackendSchema>;
export type CustomFieldType = CustomFieldDef["type"];
export type CustomFieldDef = z.infer<typeof CustomFieldDefSchema>;
export type LinearConfig = z.infer<typeof LinearConfigSchema>;
export type PipelineConfig = z.infer<typeof PipelineConfigSchema>;
export type IssueSchemaConfig = z.infer<typeof IssueSchemaConfigSchema>;
export type Config = z.infer<typeof ConfigSchema>;
export type PartialConfig = z.infer<typeof PartialConfigSchema>;
