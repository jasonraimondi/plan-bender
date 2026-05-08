package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProseList is a []string that tolerates YAML list items written as
// `- some prose: more prose` without surrounding quotes. yaml.v3 parses such
// items as a single-key mapping, not a string, which causes the strict
// decoder to reject the whole file. PRD/issue authors regularly write prose
// containing colons (e.g. "M1: introduce ..."), so the lenient unmarshaller
// flattens single-key maps back into "key: value" strings to preserve intent.
//
// Round-trip safety relies on yaml.v3's default scalar emitter quoting any
// string whose plain form would re-parse ambiguously (verified by
// TestProseList_YAMLRoundTrip_QuotesUnsafe). If that emitter behavior ever
// changes, ProseList will need an explicit MarshalYAML.
type ProseList []string

// flattenMaxDepth bounds recursive descent so circular YAML anchors cannot
// exhaust the goroutine stack. Real plan files never nest beyond a couple
// of levels; 32 is well past anything an author would write by hand.
const flattenMaxDepth = 32

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *ProseList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Tag == "!!null" {
		*s = nil
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		var kind string
		switch value.Kind {
		case yaml.DocumentNode:
			kind = "document"
		case yaml.MappingNode:
			kind = "mapping"
		case yaml.ScalarNode:
			kind = "scalar"
		case yaml.AliasNode:
			kind = "alias"
		default:
			kind = fmt.Sprintf("unknown(%d)", value.Kind)
		}
		return fmt.Errorf("line %d: expected a list, got %s", value.Line, kind)
	}
	out := make([]string, 0, len(value.Content))
	for _, item := range value.Content {
		if isNull(item) {
			return fmt.Errorf("line %d: null list item not allowed", item.Line)
		}
		v, err := flattenListItem(item, 0)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	*s = out
	return nil
}

func flattenListItem(n *yaml.Node, depth int) (string, error) {
	if depth > flattenMaxDepth {
		return "", fmt.Errorf("line %d: list item nested too deeply (possible alias cycle)", n.Line)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value, nil
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := flattenListItem(n.Content[i], depth+1)
			if err != nil {
				return "", err
			}
			// `- foo:` parses as a single-key map with a null value. Render
			// just the key so the bare-key prose shape is preserved.
			if isNull(n.Content[i+1]) {
				parts = append(parts, k)
				continue
			}
			v, err := flattenListItem(n.Content[i+1], depth+1)
			if err != nil {
				return "", err
			}
			if v == "" {
				parts = append(parts, k)
			} else {
				parts = append(parts, k+": "+v)
			}
		}
		return strings.Join(parts, ", "), nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			s, err := flattenListItem(c, depth+1)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ", "), nil
	case yaml.AliasNode:
		if n.Alias == nil {
			return "", fmt.Errorf("line %d: nil alias target", n.Line)
		}
		return flattenListItem(n.Alias, depth+1)
	}
	return "", fmt.Errorf("line %d: unsupported list item kind", n.Line)
}

func isNull(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}
