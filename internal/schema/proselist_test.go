package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProseList_PlainScalars(t *testing.T) {
	in := []byte("- alpha\n- beta\n- gamma\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{"alpha", "beta", "gamma"}, got)
}

func TestProseList_QuotedScalars(t *testing.T) {
	in := []byte(`- 'alpha: with colon'` + "\n" + `- "beta: another"` + "\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{"alpha: with colon", "beta: another"}, got)
}

func TestProseList_FlattensSingleKeyMap(t *testing.T) {
	// `- foo: bar` is a single-key mapping in YAML. The lenient decoder
	// flattens it back into a "foo: bar" string so prose with colons does
	// not reject the file.
	in := []byte("- M1: introduce path-specific Stripe key v2\n- normal item\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{
		"M1: introduce path-specific Stripe key v2",
		"normal item",
	}, got)
}

func TestProseList_FlattensMultiKeyMap(t *testing.T) {
	// Block-style multi-key list-as-map. We don't expect this in real
	// authoring, but if a multi-line item happens to parse as a multi-key
	// map we fall back to comma-joining "k: v" pairs to preserve content.
	in := []byte("- a: 1\n  b: 2\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{"a: 1, b: 2"}, got)
}

func TestProseList_NestedSequenceFlattens(t *testing.T) {
	in := []byte("- [one, two, three]\n- plain\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{"one, two, three", "plain"}, got)
}

func TestProseList_EmptyValueKey(t *testing.T) {
	// `- foo:` parses as a single-key map with a null value. We render
	// just the key so nothing is dropped silently.
	in := []byte("- bare:\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Equal(t, ProseList{"bare"}, got)
}

func TestProseList_ExplicitNull(t *testing.T) {
	// A document-level null is a valid empty list (matches an absent field).
	in := []byte("~\n")
	var got ProseList
	require.NoError(t, yaml.Unmarshal(in, &got))
	assert.Nil(t, got)
}

func TestProseList_RejectsNullListItem(t *testing.T) {
	// A null *item* (e.g. `- ~`) is almost always a typo; preserving it
	// silently as "" hides the mistake.
	for _, in := range [][]byte{
		[]byte("- ~\n"),
		[]byte("- null\n"),
		[]byte("- alpha\n- ~\n- beta\n"),
	} {
		var got ProseList
		err := yaml.Unmarshal(in, &got)
		require.Error(t, err, "input: %q", in)
		assert.Contains(t, err.Error(), "null list item")
	}
}

func TestProseList_RejectsNonSequence(t *testing.T) {
	// A bare scalar where a list is expected must error — we don't want
	// silent coercion to mask malformed plan files.
	in := []byte("just a string\n")
	var got ProseList
	err := yaml.Unmarshal(in, &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected a list")
}

func TestProseList_JSONRoundTrip(t *testing.T) {
	// ProseList must JSON-encode the same as []string so CLI status
	// output and context plumbing are unaffected by the type swap.
	src := ProseList{"alpha", "beta: with colon"}
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `["alpha","beta: with colon"]`, string(b))

	var back ProseList
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, src, back)
}

func TestProseList_YAMLRoundTrip_QuotesUnsafe(t *testing.T) {
	// After flattening, the string contains "key: value" — the marshaller
	// must quote it so the round-trip is stable rather than re-creating
	// the same single-key-map ambiguity.
	src := ProseList{"M1: introduce v2 key", "plain item"}
	out, err := yaml.Marshal(src)
	require.NoError(t, err)

	var back ProseList
	require.NoError(t, yaml.Unmarshal(out, &back))
	assert.Equal(t, src, back)
}

func TestPrdYaml_LenientUnmarshal(t *testing.T) {
	// Whole-PRD smoke test: unquoted prose-with-colon list items in
	// every prose ProseList field decode without error.
	in := []byte(`name: Test
slug: test
status: active
created: "2026-03-26"
updated: "2026-03-26"
description: desc
why: why
outcome: outcome
in_scope:
  - M1: do the thing
  - regular bullet
out_of_scope:
  - skipped: with reason
decisions:
  - 'Decision: explicitly quoted'
  - Decision: unquoted prose
risks:
  - Risk: cutover edge case
validation:
  - 'V1: assertion'
open_questions:
  - Q1: still unresolved
`)
	var prd PrdYaml
	require.NoError(t, yaml.Unmarshal(in, &prd))
	assert.Equal(t, ProseList{"M1: do the thing", "regular bullet"}, prd.InScope)
	assert.Equal(t, ProseList{"skipped: with reason"}, prd.OutOfScope)
	assert.Equal(t, ProseList{"Decision: explicitly quoted", "Decision: unquoted prose"}, prd.Decisions)
	assert.Equal(t, ProseList{"Risk: cutover edge case"}, prd.Risks)
	assert.Equal(t, ProseList{"V1: assertion"}, prd.Validation)
	assert.Equal(t, ProseList{"Q1: still unresolved"}, prd.OpenQuestions)
}

func TestIssueYaml_LenientUnmarshal(t *testing.T) {
	in := []byte(`id: 1
slug: x
name: X
track: intent
status: todo
priority: high
points: 1
labels: []
blocked_by: []
blocking: []
created: "2026-01-01"
updated: "2026-01-02"
outcome: ok
scope: small
acceptance_criteria:
  - AC1: behavior holds
steps:
  - some/path/file.ts: implement methodName
  - already-quoted step
use_cases:
  - UC-1
`)
	var iss IssueYaml
	require.NoError(t, yaml.Unmarshal(in, &iss))
	assert.Equal(t, ProseList{"AC1: behavior holds"}, iss.AcceptanceCriteria)
	assert.Equal(t, ProseList{
		"some/path/file.ts: implement methodName",
		"already-quoted step",
	}, iss.Steps)
	assert.Equal(t, ProseList{"UC-1"}, iss.UseCases)
}
