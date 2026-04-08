package config

// merge applies a PartialConfig layer on top of a Config base.
// Semantics: agents merge per-key, other maps merge keys, slices replace entirely, scalars overwrite.
func merge(base Config, layer PartialConfig) Config {
	out := base

	if layer.Tracks != nil {
		out.Tracks = layer.Tracks
	}
	if layer.WorkflowStates != nil {
		out.WorkflowStates = layer.WorkflowStates
	}
	if layer.PlansDir != nil {
		out.PlansDir = *layer.PlansDir
	}
	if layer.MaxPoints != nil {
		out.MaxPoints = *layer.MaxPoints
	}
	if layer.Agents != nil {
		// Per-key merge: copy existing entries then apply layer entries.
		// Always create a new map to avoid mutating base.rawAgents.
		newMap := make(map[string]*AgentEntry, len(out.rawAgents)+len(layer.Agents))
		for k, v := range out.rawAgents {
			newMap[k] = v
		}
		for k, v := range layer.Agents {
			newMap[k] = v
		}
		out.rawAgents = newMap
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
	if layer.ReviewWithUser != nil {
		out.ReviewWithUser = layer.ReviewWithUser
	}

	return out
}

func mergeLinear(base *LinearConfig, layer *LinearConfig) {
	if layer.Enabled {
		base.Enabled = true
	}
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
