package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewSyncCmd creates the sync command with push/pull subcommands.
func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync issues with backend (Linear)",
	}

	cmd.AddCommand(newSyncPushCmd(), newSyncPullCmd())
	return cmd
}

func newSyncPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <slug>",
		Short: "Push local issues to remote backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncPush(cmd, args[0])
		},
	}
}

func newSyncPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <slug>",
		Short: "Pull remote state to local YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncPull(cmd, args[0])
		},
	}
}

func syncPush(cmd *cobra.Command, slug string) error {
	root, _ := os.Getwd()
	ctx := context.Background()

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	be, err := backend.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}

	// Read PRD
	prdPath := filepath.Join(cfg.PlansDir, slug, "prd.yaml")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		return fmt.Errorf("reading PRD: %w", err)
	}
	var prd schema.PrdYaml
	if err := yaml.Unmarshal(prdData, &prd); err != nil {
		return fmt.Errorf("parsing PRD: %w", err)
	}

	// Ensure project exists
	projectID := ""
	if prd.Linear != nil && prd.Linear.ProjectID != "" {
		projectID = prd.Linear.ProjectID
	} else {
		project, err := be.CreateProject(ctx, &prd)
		if err != nil {
			return fmt.Errorf("creating project: %w", err)
		}
		projectID = project.ID
		if prd.Linear == nil {
			prd.Linear = &schema.LinearRef{}
		}
		prd.Linear.ProjectID = projectID
		prdOut, _ := yaml.Marshal(&prd)
		if err := atomicWriteFile(prdPath, prdOut, 0o644); err != nil {
			return fmt.Errorf("writing PRD: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created project %s\n", projectID)
	}

	// Push issues
	issues, err := loadIssues(cfg.PlansDir, slug)
	if err != nil {
		return err
	}

	created, updated := 0, 0
	for i := range issues {
		issue := &issues[i]
		if issue.LinearID != nil && *issue.LinearID != "" {
			// Update existing
			_, err := be.UpdateIssue(ctx, issue)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error updating #%d: %v\n", issue.ID, err)
				continue
			}
			updated++
		} else {
			// Create new
			remote, err := be.CreateIssue(ctx, issue, projectID)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error creating #%d: %v\n", issue.ID, err)
				continue
			}
			// Write linear_id back
			remoteID := remote.ID
			issue.LinearID = &remoteID
			issueData, _ := yaml.Marshal(issue)
			issuePath := filepath.Join(cfg.PlansDir, slug, "issues",
				fmt.Sprintf("%d-%s.yaml", issue.ID, issue.Slug))
			if err := atomicWriteFile(issuePath, issueData, 0o644); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error writing #%d: %v\n", issue.ID, err)
			}
			created++
		}
	}

	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status":  "ok",
			"created": created,
			"updated": updated,
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "push complete: %d created, %d updated\n", created, updated)
	return nil
}

func syncPull(cmd *cobra.Command, slug string) error {
	root, _ := os.Getwd()
	ctx := context.Background()

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	be, err := backend.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}

	// Read PRD for project ID
	prdPath := filepath.Join(cfg.PlansDir, slug, "prd.yaml")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		return fmt.Errorf("reading PRD: %w", err)
	}
	var prd schema.PrdYaml
	if err := yaml.Unmarshal(prdData, &prd); err != nil {
		return fmt.Errorf("parsing PRD: %w", err)
	}

	if prd.Linear == nil || prd.Linear.ProjectID == "" {
		return fmt.Errorf("no linear.project_id in PRD — run push first")
	}

	result, err := be.PullProject(ctx, prd.Linear.ProjectID)
	if err != nil {
		return fmt.Errorf("pulling project: %w", err)
	}

	// Read local issues and update from remote
	issues, err := loadIssues(cfg.PlansDir, slug)
	if err != nil {
		return err
	}

	// Build remote index by ID
	remoteByID := make(map[string]backend.RemoteIssue)
	for _, ri := range result.Issues {
		remoteByID[ri.ID] = ri
	}

	changed := 0
	for i := range issues {
		issue := &issues[i]
		if issue.LinearID == nil || *issue.LinearID == "" {
			continue
		}
		remote, ok := remoteByID[*issue.LinearID]
		if !ok {
			continue
		}

		// Compare and update
		dirty := false
		if remote.Status != "" && remote.Status != issue.Status {
			issue.Status = remote.Status
			dirty = true
		}
		if remote.Priority != "" && remote.Priority != issue.Priority {
			issue.Priority = remote.Priority
			dirty = true
		}
		if remote.Assignee != "" {
			assignee := remote.Assignee
			if issue.Assignee == nil || *issue.Assignee != assignee {
				issue.Assignee = &assignee
				dirty = true
			}
		}

		if dirty {
			issueData, _ := yaml.Marshal(issue)
			issuePath := filepath.Join(cfg.PlansDir, slug, "issues",
				fmt.Sprintf("%d-%s.yaml", issue.ID, issue.Slug))
			if err := atomicWriteFile(issuePath, issueData, 0o644); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error writing #%d: %v\n", issue.ID, err)
				continue
			}
			changed++
		}
	}

	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status":  "ok",
			"updated": changed,
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "pull complete: %d issues updated\n", changed)
	return nil
}
