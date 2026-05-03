package planrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// Commit applies in-session mutations to disk under the held plan lock. It
// runs preflight (marshal, validate, filename plan) before touching any
// file: if any preflight check fails, no writes happen. During the write
// phase, files are written atomically per file in deterministic order; if
// a write fails partway through, prior writes are best-effort rolled back
// to their original bytes (or removed if they didn't exist before).
//
// Commit does not release the plan lock. Callers must Close the session
// when done; sync.Once makes that safe to call regardless of Commit
// outcome.
func (s *PlanSession) Commit(cfg config.Config) error {
	if s.closed {
		return ErrSessionClosed
	}
	plan, err := s.buildCommitPlan(cfg)
	if err != nil {
		return err
	}
	if err := applyCommitPlan(s.plans.adapters, plan); err != nil {
		return err
	}
	s.markClean()
	return nil
}

// fileWrite is one staged write in a commit plan. prevData holds the bytes
// that were on disk at write time; existed records whether the file was
// present before the write (so rollback can choose between restore and
// remove).
type fileWrite struct {
	path     string
	data     []byte
	perm     fs.FileMode
	prevData []byte
	existed  bool
}

// fileRemove is one staged removal — used when an issue's slug changes and
// the old filename must be cleaned up. prevData lets rollback restore the
// removed file.
type fileRemove struct {
	path     string
	prevData []byte
	perm     fs.FileMode
}

type commitPlan struct {
	writes  []fileWrite
	removes []fileRemove
	// dirsToEnsure are directories that must exist before any write runs.
	dirsToEnsure []string
}

// buildCommitPlan runs every preflight check and returns the staged plan.
// On any failure the plan is discarded and no disk I/O has happened.
func (s *PlanSession) buildCommitPlan(cfg config.Config) (commitPlan, error) {
	plan := commitPlan{}

	// 1. Validate the in-session snapshot against the schema package.
	res := validateSnapshot(s.snapshot, s.baselineFilenames, cfg)
	if !res.Valid {
		return commitPlan{}, &CommitValidationError{Result: res}
	}

	planDir := filepath.Join(s.plans.plansDir, s.slug)
	issuesDir := filepath.Join(planDir, "issues")
	plan.dirsToEnsure = []string{planDir, issuesDir}

	// 2. Marshal + roundtrip-check the PRD if dirty.
	if s.dirtyPRD {
		data, err := marshalAndProbe(&s.snapshot.PRD, func(b []byte) error {
			var probe schema.PrdYaml
			return strictUnmarshal(b, &probe)
		})
		if err != nil {
			return commitPlan{}, fmt.Errorf("marshal prd: %w", err)
		}
		plan.writes = append(plan.writes, fileWrite{
			path: filepath.Join(planDir, "prd.yaml"),
			data: data,
			perm: 0o644,
		})
	}

	// 3. Marshal + roundtrip-check each dirty issue and stage writes
	//    (plus removes for slug renames). Iterate by sorted ID for
	//    deterministic write order.
	dirtyIDs := make([]int, 0, len(s.dirtyIssues))
	for id := range s.dirtyIssues {
		dirtyIDs = append(dirtyIDs, id)
	}
	sort.Ints(dirtyIDs)

	seenFilenames := map[string]int{}
	for _, id := range dirtyIDs {
		iss := s.findIssueByID(id)
		if iss == nil {
			return commitPlan{}, fmt.Errorf("commit plan: dirty issue id #%d missing from snapshot", id)
		}
		filename := canonicalIssueFilename(iss)
		if other, ok := seenFilenames[filename]; ok {
			return commitPlan{}, fmt.Errorf("commit plan: filename %q produced by both id #%d and id #%d", filename, id, other)
		}
		seenFilenames[filename] = id

		data, err := marshalAndProbe(iss, func(b []byte) error {
			var probe schema.IssueYaml
			return strictUnmarshal(b, &probe)
		})
		if err != nil {
			return commitPlan{}, fmt.Errorf("marshal issue #%d: %w", id, err)
		}

		newPath := filepath.Join(issuesDir, filename)
		plan.writes = append(plan.writes, fileWrite{
			path: newPath,
			data: data,
			perm: 0o644,
		})

		// If this issue existed at Open time and the canonical filename
		// changed (slug rename), schedule the old file for removal so
		// the on-disk dir doesn't keep both.
		if oldName, ok := s.baselineFilenames[id]; ok && oldName != filename {
			plan.removes = append(plan.removes, fileRemove{
				path: filepath.Join(issuesDir, oldName),
				perm: 0o644,
			})
		}
	}

	// 4. Capture pre-write bytes so the apply phase can roll back. We do
	//    this in preflight (still no writes) so a missing-file read isn't
	//    confused with a half-applied state later.
	for i := range plan.writes {
		w := &plan.writes[i]
		data, existed, err := readIfExists(w.path)
		if err != nil {
			return commitPlan{}, fmt.Errorf("preflight read %s: %w", w.path, err)
		}
		w.prevData = data
		w.existed = existed
	}
	for i := range plan.removes {
		r := &plan.removes[i]
		data, existed, err := readIfExists(r.path)
		if err != nil {
			return commitPlan{}, fmt.Errorf("preflight read %s: %w", r.path, err)
		}
		// A scheduled remove targets a baseline filename we just rewrote
		// elsewhere; if the file vanished between Open and Commit we
		// drop the remove silently rather than failing the whole commit.
		if !existed {
			plan.removes[i].prevData = nil
			continue
		}
		r.prevData = data
	}

	return plan, nil
}

// applyCommitPlan executes writes then removes. On any failure it best-effort
// reverses the operations that already succeeded so the plan dir lands as
// close to the pre-commit state as possible.
func applyCommitPlan(adapters Adapters, plan commitPlan) error {
	for _, dir := range plan.dirsToEnsure {
		if err := adapters.Mkdir(dir, 0o755); err != nil {
			return fmt.Errorf("ensure dir %s: %w", dir, err)
		}
	}

	type undo func() error
	var undos []undo

	// rollback runs the undos in reverse and returns any errors it
	// collected so the caller can join them with the originating
	// failure. Best-effort means we keep going past per-undo errors
	// rather than stopping at the first one — but we still surface
	// what failed instead of silently swallowing it.
	rollback := func() []error {
		var errs []error
		for i := len(undos) - 1; i >= 0; i-- {
			if err := undos[i](); err != nil {
				errs = append(errs, fmt.Errorf("rollback step %d: %w", i, err))
			}
		}
		return errs
	}

	for _, w := range plan.writes {
		w := w
		if err := adapters.Write(w.path, w.data, w.perm); err != nil {
			writeErr := fmt.Errorf("write %s: %w", w.path, err)
			return errors.Join(append([]error{writeErr}, rollback()...)...)
		}
		if w.existed {
			prev := w.prevData
			path := w.path
			perm := w.perm
			undos = append(undos, func() error {
				return adapters.Write(path, prev, perm)
			})
		} else {
			path := w.path
			undos = append(undos, func() error { return adapters.Remove(path) })
		}
	}

	for _, r := range plan.removes {
		r := r
		if r.prevData == nil {
			// File already absent at preflight time; nothing to do.
			continue
		}
		if err := adapters.Remove(r.path); err != nil {
			removeErr := fmt.Errorf("remove %s: %w", r.path, err)
			return errors.Join(append([]error{removeErr}, rollback()...)...)
		}
		prev := r.prevData
		path := r.path
		perm := r.perm
		undos = append(undos, func() error {
			return adapters.Write(path, prev, perm)
		})
	}

	return nil
}

// markClean clears in-session dirty state and refreshes the baseline
// filename map so a subsequent UpdateIssue → Commit cycle on the same
// session sees the just-committed names as baseline.
func (s *PlanSession) markClean() {
	s.dirtyPRD = false
	s.dirtyIssues = map[int]bool{}
	for i := range s.snapshot.Issues {
		iss := &s.snapshot.Issues[i]
		s.baselineFilenames[iss.ID] = canonicalIssueFilename(iss)
	}
}

func (s *PlanSession) findIssueByID(id int) *schema.IssueYaml {
	for i := range s.snapshot.Issues {
		if s.snapshot.Issues[i].ID == id {
			return &s.snapshot.Issues[i]
		}
	}
	return nil
}

// marshalAndProbe marshals v and re-parses the bytes through probe to catch
// non-roundtripping output (e.g. duplicate keys from a future custom
// MarshalYAML). The probe runs before any disk write so a regression here
// surfaces as a preflight error rather than corrupting on-disk YAML.
func marshalAndProbe(v any, probe func([]byte) error) ([]byte, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	if err := probe(data); err != nil {
		return nil, fmt.Errorf("roundtrip check: %w", err)
	}
	return data, nil
}

// readIfExists returns (bytes, true, nil) when path exists, (nil, false, nil)
// when it doesn't, and (nil, false, err) for any other error. Used during
// commit preflight so rollback can restore the original bytes.
func readIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

// CommitValidationError wraps the schema package's PlanValidationResult so
// callers can distinguish preflight validation failure from other commit
// errors (marshal, write, etc.) without parsing error strings.
type CommitValidationError struct {
	Result schema.PlanValidationResult
}

func (e *CommitValidationError) Error() string {
	return fmt.Sprintf("commit preflight validation failed for plan %q", e.Result.PRD.File)
}
