package reports

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"mybuilds/internal/protocol"
)

func TestParseJUnitActualCasesAndNestedCounts(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><testsuites tests="5" failures="1" errors="1" skipped="1" time="900"><testsuite tests="2"><testcase name="pass" time="0.1"/><testcase time="0.2"/></testsuite><testsuite tests="3"><testcase name="中文失败" time="0.300000001"><failure message="expected mismatch">actual body</failure></testcase><testcase name="error" time="0.4"><error message="fixture error"/></testcase><testcase time="0"><skipped/></testcase></testsuite></testsuites>`
	result, err := ParseJUnit(context.Background(), strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.JUnitCounts{Tests: 5, Failures: 1, Errors: 1, Skipped: 1, DurationNS: 1000000001}
	if result.Counts != want {
		t.Fatalf("counts = %+v", result.Counts)
	}
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Case != "中文失败" || result.Diagnostics[0].Outcome != "failure" || result.Diagnostics[0].PathKey != "" || !strings.Contains(result.Diagnostics[0].Message, "actual body") {
		t.Fatalf("diagnostics = %+v", result.Diagnostics)
	}
}

func TestParseJUnitAllowedStructuresAndZeroCases(t *testing.T) {
	for _, input := range []string{
		`<testsuite tests="0" failures="0" errors="0" skipped="0"/>`,
		`<testsuites><testsuite><testsuite/></testsuite></testsuites>`,
		`<testsuite metadata="allowed"><properties><property name="x" value="y"/></properties><system-out>text</system-out><testcase><properties><property>text</property></properties><system-err>diagnostic</system-err></testcase></testsuite>`,
		"\xef\xbb\xbf<?xml version='1.0'?><testsuite/>",
	} {
		result, err := ParseJUnit(context.Background(), strings.NewReader(input), nil)
		if err != nil {
			t.Fatalf("legal fixture rejected: %v", err)
		}
		if result.Diagnostics == nil {
			t.Fatal("diagnostics must be nonnull")
		}
	}
}

func TestParseJUnitRejectsInvalidSemantics(t *testing.T) {
	inputs := []string{
		``, `<testcase/>`, `<testsuite/><testsuite/>`, `<testsuite>unexpected</testsuite>`,
		`<testsuites><testcase/></testsuites>`, `<testsuite><unknown/></testsuite>`, `<testsuite><testcase><testcase/></testcase></testsuite>`,
		`<testsuite tests="2"><testcase/></testsuite>`, `<testsuite failures="0"><testcase><failure/></testcase></testsuite>`,
		`<testsuite><testcase><failure/><error/></testcase></testsuite>`, `<testsuite><testcase><skipped/><skipped/></testcase></testsuite>`,
		`<testsuite><testcase name="one" name="two"/></testsuite>`, `<testsuite tests="-1"/>`, `<testsuite tests="1.0"/>`,
		`<testsuite><testcase time="-1"/></testsuite>`, `<testsuite><testcase time="1e3"/></testsuite>`, `<testsuite><testcase time="NaN"/></testsuite>`, `<testsuite><testcase time="Inf"/></testsuite>`, `<testsuite><testcase time="0.0000000001"/></testsuite>`,
		`<testsuite xmlns="urn:bad"/>`, `<testsuite xmlns:p="urn:bad"/>`, `<p:testsuite/>`, `<testsuite xml:space="preserve"/>`,
		`<!DOCTYPE testsuite [<!ENTITY x SYSTEM "file:///forbidden">]><testsuite/>`, `<testsuite><system-out>&unknown;</system-out></testsuite>`,
		`<?unsafe ignored?><testsuite/>`, ` <?xml version="1.0"?><testsuite/>`, `<?xml version="1.1"?><testsuite/>`, `<?xml version="1.0" encoding="ISO-8859-1"?><testsuite/>`, `<?xml version="1.0" version="1.0"?><testsuite/>`, `<?xml version="1.0" standalone="other"?><testsuite/>`, `<testsuite/><?xml version="1.0"?>`,
	}
	for i, input := range inputs {
		_, err := ParseJUnit(context.Background(), strings.NewReader(input), nil)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("fixture %d: expected invalid, got %v", i, err)
		}
	}
}

func TestParseJUnitRejectsSecretsIncludingSplitDecodedText(t *testing.T) {
	for _, input := range []string{
		`<testsuite marker="secret"/>`, `<testsuite marker="s&#101;cret"/>`,
		`<testsuite><testcase><failure>sec<![CDATA[ret]]></failure></testcase></testsuite>`,
		`<testsuite><system-out>sec<!-- ignored -->ret</system-out></testsuite>`,
		`<testsuite><properties><property>s&#101;cret</property></properties></testsuite>`,
		`<testsuite><testcase name="s&#101;cret"/></testsuite>`,
	} {
		_, err := ParseJUnit(context.Background(), strings.NewReader(input), []string{"", "secret"})
		if !errors.Is(err, ErrSecret) || err != ErrSecret {
			t.Fatalf("expected fixed secret error, got %v", err)
		}
	}
}

func TestParseJUnitValidatesPastDiagnosticTruncation(t *testing.T) {
	prefix := `<testsuite>` + strings.Repeat(`<testcase><failure>visible</failure></testcase>`, 20)
	for _, tail := range []struct {
		xml  string
		want error
	}{
		{`<testcase><failure>sec<![CDATA[ret]]></failure></testcase></testsuite>`, ErrSecret},
		{`<testcase><failure/><failure/></testcase></testsuite>`, ErrInvalid},
	} {
		_, err := ParseJUnit(context.Background(), strings.NewReader(prefix+tail.xml), []string{"secret"})
		if !errors.Is(err, tail.want) {
			t.Fatalf("late boundary = %v", err)
		}
	}
	input := `<testsuite>` + strings.Repeat(`<testcase name="中文&#9;名字"><failure message="x&#10;y">`+strings.Repeat("中", 1000)+`</failure></testcase>`, 30) + `</testsuite>`
	result, err := ParseJUnit(context.Background(), strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Counts.Tests != 30 || result.Counts.Failures != 30 || len(result.Diagnostics) > 20 {
		t.Fatal("truncation changed counts")
	}
	size := 0
	for _, d := range result.Diagnostics {
		size += len(d.PathKey) + len(d.Case) + len(d.Outcome) + len(d.Message)
		if len(d.Message) > 1024 || !utf8.ValidString(d.Message) || strings.ContainsFunc(d.Message, unicode.IsControl) || strings.ContainsFunc(d.Case, unicode.IsControl) {
			t.Fatal("unsafe diagnostic")
		}
	}
	if size > 20*1024 {
		t.Fatal("diagnostic bytes exceeded")
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("reader private text") }
func TestParseJUnitSafeReaderAndCancellation(t *testing.T) {
	_, err := ParseJUnit(context.Background(), failedReader{}, nil)
	if !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "private") {
		t.Fatalf("reader error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ParseJUnit(ctx, strings.NewReader(`<testsuite/>`), nil)
	if !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
	_, err = ParseJUnit(context.Background(), io.NopCloser(strings.NewReader(`<testsuite>`)), nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestMergeJUnitCountsAndWholeInputValidation(t *testing.T) {
	one := protocol.JUnitResult{Counts: protocol.JUnitCounts{Tests: 1, Failures: 1, DurationNS: 10}, Diagnostics: []protocol.JUnitDiagnostic{{PathKey: strings.Repeat("a", 64), Case: "a", Outcome: "failure", Message: "first"}}}
	two := protocol.JUnitResult{Counts: protocol.JUnitCounts{Tests: 1, Errors: 1, DurationNS: 20}, Diagnostics: []protocol.JUnitDiagnostic{{PathKey: strings.Repeat("b", 64), Case: "b", Outcome: "error", Message: "second"}}}
	got, err := MergeJUnit([]protocol.JUnitResult{one, two})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts != (protocol.JUnitCounts{Tests: 2, Failures: 1, Errors: 1, DurationNS: 30}) || !reflect.DeepEqual(got.Diagnostics, append(one.Diagnostics, two.Diagnostics...)) {
		t.Fatal("merge changed ordering or totals")
	}
	many := make([]protocol.JUnitResult, 21)
	for i := range many {
		many[i] = one
	}
	many[20].Counts.Tests = -1
	if _, err = MergeJUnit(many); !errors.Is(err, ErrInvalid) {
		t.Fatal("late invalid aggregate accepted")
	}
	empty, err := MergeJUnit(nil)
	if err != nil || empty.Diagnostics == nil {
		t.Fatal("empty aggregate must be explicit")
	}
}
