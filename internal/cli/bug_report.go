package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeBugReport writes a pb-error-report-<UTC>.log to root capturing a failed
// command and its error, returning the report path. It is the code-side
// counterpart to the report_bugs prompt the skills inject into sub-agents: a
// dispatch-orchestration failure happens in Go before any sub-agent runs, so
// only the failing command can produce the artifact. Best-effort — the caller
// surfaces a write failure as a warning, not a hard error.
func writeBugReport(root, version, command string, cause error) (string, error) {
	now := time.Now().UTC()
	name := fmt.Sprintf("pb-error-report-%s.log", now.Format("20060102T150405Z"))
	path := filepath.Join(root, name)
	body := fmt.Sprintf(`plan-bender bug report

command: pba %s
version: %s
time:    %s

error:
%s

File this at https://github.com/jasonraimondi/plan-bender/issues
`, command, version, now.Format(time.RFC3339), cause.Error())
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
