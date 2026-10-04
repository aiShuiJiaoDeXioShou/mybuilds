package reports

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseJUnitChecksCompleteDisplayedDiagnostics(t *testing.T) {
	for _, xml := range []string{
		`<testsuite><testcase><failure message="red">blue</failure></testcase></testsuite>`,
		`<testsuite><testcase name="red&#10;blue"><failure/></testcase></testsuite>`,
		`<testsuite><testcase><failure message="red&#9;blue"/></testcase></testsuite>`,
		`<testsuite><testcase><failure>` + strings.Repeat("x", 1200) + `red&#10;blue</failure></testcase></testsuite>`,
	} {
		_, err := ParseJUnit(context.Background(), strings.NewReader(xml), []string{"red blue"})
		if !errors.Is(err, ErrSecret) {
			t.Fatalf("displayed secret accepted: %v", err)
		}
	}
}
