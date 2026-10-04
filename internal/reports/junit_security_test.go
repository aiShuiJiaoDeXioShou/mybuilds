package reports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
)

func TestMergeJUnitPathKeyUsesDisplayCap(t *testing.T) {
	input := `<testsuite>` + strings.Repeat(`<testcase name="`+strings.Repeat("c", 512)+`"><failure>`+strings.Repeat("m", 1024)+`</failure></testcase>`, 13) + `</testsuite>`
	parsed, err := ParseJUnit(context.Background(), strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Diagnostics) != 13 {
		t.Fatalf("parser diagnostics %d", len(parsed.Diagnostics))
	}
	for i := range parsed.Diagnostics {
		parsed.Diagnostics[i].PathKey = strings.Repeat("a", 64)
	}
	merged, err := MergeJUnit([]protocol.JUnitResult{parsed})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Counts.Tests != 13 || merged.Counts.Failures != 13 || len(merged.Diagnostics) != 12 {
		t.Fatalf("merge changed counts or display cap: %+v", merged.Counts)
	}
	total := 0
	for _, d := range merged.Diagnostics {
		total += diagnosticBytes(d)
	}
	if total > maxDiagnosticBytes {
		t.Fatal("diagnostic bytes exceed cap")
	}
	parsed.Diagnostics[12].Message = string([]byte{0xff})
	if _, err = MergeJUnit([]protocol.JUnitResult{parsed}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("late invalid diagnostic: %v", err)
	}
}

func TestParseJUnitExactBoundaries(t *testing.T) {
	attrs := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, ` a%d="v"`, i)
		}
		return b.String()
	}
	fixtures := []struct {
		name, xml string
		want      error
	}{
		{"bytes", `<testsuite><system-out>` + strings.Repeat("x", maxXMLBytes-len(`<testsuite><system-out></system-out></testsuite>`)) + `</system-out></testsuite>`, nil},
		{"bytes-plus-one", `<testsuite><system-out>` + strings.Repeat("x", maxXMLBytes+1-len(`<testsuite><system-out></system-out></testsuite>`)) + `</system-out></testsuite>`, ErrLimit},
		{"depth", strings.Repeat(`<testsuite>`, 64) + strings.Repeat(`</testsuite>`, 64), nil},
		{"depth-plus-one", strings.Repeat(`<testsuite>`, 65) + strings.Repeat(`</testsuite>`, 65), ErrLimit},
		{"attributes", `<testsuite` + attrs(64) + `/>`, nil},
		{"attributes-plus-one", `<testsuite` + attrs(65) + `/>`, ErrLimit},
		{"attribute-name", `<testsuite ` + strings.Repeat("a", 256) + `="v"/>`, nil},
		{"attribute-name-plus-one", `<testsuite ` + strings.Repeat("a", 257) + `="v"/>`, ErrLimit},
		{"attribute-value", `<testsuite a="` + strings.Repeat("v", 4096) + `"/>`, nil},
		{"attribute-value-plus-one", `<testsuite a="` + strings.Repeat("v", 4097) + `"/>`, ErrLimit},
		{"case-name", `<testsuite><testcase name="` + strings.Repeat("n", 512) + `"/></testsuite>`, nil},
		{"case-name-plus-one", `<testsuite><testcase name="` + strings.Repeat("n", 513) + `"/></testsuite>`, ErrLimit},
		{"case-time", `<testsuite><testcase time="86400"/></testsuite>`, nil},
		{"case-time-plus-one", `<testsuite><testcase time="86400.000000001"/></testsuite>`, ErrLimit},
		{"total-time", `<testsuite>` + strings.Repeat(`<testcase time="86400"/>`, 365) + `</testsuite>`, nil},
		{"total-time-plus-one", `<testsuite>` + strings.Repeat(`<testcase time="86400"/>`, 365) + `<testcase time="0.000000001"/></testsuite>`, ErrLimit},
		{"case-count", `<testsuite>` + strings.Repeat(`<testcase/>`, 100000) + `</testsuite>`, nil},
		{"case-count-plus-one", `<testsuite>` + strings.Repeat(`<testcase/>`, 100001) + `</testsuite>`, ErrLimit},
		{"invalid-utf8", "<testsuite><system-out>" + string([]byte{0xff}) + "</system-out></testsuite>", ErrInvalid},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			_, err := ParseJUnit(context.Background(), strings.NewReader(f.xml), nil)
			if !errors.Is(err, f.want) {
				t.Fatalf("boundary result: %v", err)
			}
		})
	}
}
