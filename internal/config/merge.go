package config

// merge applies a PartialConfig layer on top of a Config base.
// Semantics: maps merge keys, slices replace entirely, scalars overwrite.
func merge(base Config, layer PartialConfig) Config {
	out := base

	if layer.Backend != nil {
		out.Backend = *layer.Backend
	}
	if layer.Tracks != nil {
		out.Tracks = layer.Tracks
	}
	if layer.WorkflowStates != nil {
		out.WorkflowStates = layer.WorkflowStates
	}
	if layer.StepPattern != nil {
		out.StepPattern = *layer.StepPattern
	}
	if layer.PlansDir != nil {
		out.PlansDir = *layer.PlansDir
	}
	if layer.MaxPoints != nil {
		out.MaxPoints = *layer.MaxPoints
	}
	if layer.InstallTarget != nil {
		out.InstallTarget = *layer.InstallTarget
	}
	if layer.UpdateCheck != nil {
		out.UpdateCheck = *layer.UpdateCheck
	}
	if layer.Pipeline != nil {
		out.Pipeline.Skip = layer.Pipeline.Skip
	}
	if layer.IssueSchema != nil {
		out.IssueSchema.CustomFields = layer.IssueSchema.CustomFields
	}
	if layer.Linear != nil {
		mergeLinear(&out.Linear, layer.Linear)
	}

	return out
}

func mergeLinear(base *LinearConfig, layer *LinearConfig) {
	if layer.APIKey != "" {
		base.APIKey = layer.APIKey
	}
	if layer.Team != "" {
		base.Team = layer.Team
	}
	if layer.ProjectID != "" {
		base.ProjectID = layer.ProjectID
	}
	if layer.StatusMap != nil {
		if base.StatusMap == nil {
			base.StatusMap = make(map[string]string)
		}
		for k, v := range layer.StatusMap {
			base.StatusMap[k] = v
		}
	}
}
