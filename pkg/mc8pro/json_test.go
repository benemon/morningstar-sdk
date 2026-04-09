package mc8pro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestJSONRoundTrip is the gating test for Phase 1. For each fixture,
// it loads a real Morningstar editor backup, unmarshals it into our
// [Dump] type, re-marshals it, and verifies the result is semantically
// identical to the original.
//
// "Semantically identical" means: parse both sides as untyped
// map[string]interface{} and compare with reflect.DeepEqual. This
// ignores whitespace, key ordering, and JSON number formatting
// differences, but catches any field we dropped, renamed, or invented.
func TestJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		dumpType string
	}{
		{"single bank", "bank-guitar-live.json", "singleBank"},
		{"all banks", "all-banks.json", "allBanks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.fixture)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			var dump Dump
			if err := json.Unmarshal(original, &dump); err != nil {
				t.Fatalf("unmarshal into Dump: %v", err)
			}
			if dump.DumpType != tc.dumpType {
				t.Fatalf("expected dumpType=%q, got %q", tc.dumpType, dump.DumpType)
			}

			remarshaled, err := json.Marshal(&dump)
			if err != nil {
				t.Fatalf("marshal Dump: %v", err)
			}

			var origAny, newAny any
			if err := json.Unmarshal(original, &origAny); err != nil {
				t.Fatalf("unmarshal original to any: %v", err)
			}
			if err := json.Unmarshal(remarshaled, &newAny); err != nil {
				t.Fatalf("unmarshal remarshaled to any: %v", err)
			}

			if !reflect.DeepEqual(origAny, newAny) {
				// On mismatch, produce a targeted diff so the failing
				// field is discoverable without eyeballing MBs of JSON.
				diff := firstDiff("", origAny, newAny)
				t.Fatalf("round-trip mismatch:\n%s", diff)
			}
		})
	}
}

// firstDiff walks two JSON-like trees in parallel and returns a
// human-readable description of the first divergence. This is much
// more actionable than reflect.DeepEqual's implicit "not equal".
func firstDiff(path string, a, b any) string {
	if reflect.DeepEqual(a, b) {
		return ""
	}
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return path + ": type mismatch (want map, got " + describeType(b) + ")"
		}
		// Check for keys only in one side.
		for k := range av {
			if _, ok := bv[k]; !ok {
				return path + "." + k + ": present in original, missing after round-trip"
			}
		}
		for k := range bv {
			if _, ok := av[k]; !ok {
				return path + "." + k + ": introduced by round-trip, not in original"
			}
		}
		for k, va := range av {
			if d := firstDiff(path+"."+k, va, bv[k]); d != "" {
				return d
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return path + ": type mismatch (want array, got " + describeType(b) + ")"
		}
		if len(av) != len(bv) {
			return path + ": length mismatch (original " + itoa(len(av)) + ", round-trip " + itoa(len(bv)) + ")"
		}
		for i := range av {
			if d := firstDiff(path+"["+itoa(i)+"]", av[i], bv[i]); d != "" {
				return d
			}
		}
	default:
		return path + ": value mismatch (original " + sprint(a) + ", round-trip " + sprint(b) + ")"
	}
	return "" // should be unreachable if DeepEqual said "different"
}

func describeType(v any) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// Tiny helpers so this file stays in the testing package without
// pulling in fmt for formatting one-off error strings.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}

func sprint(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}
