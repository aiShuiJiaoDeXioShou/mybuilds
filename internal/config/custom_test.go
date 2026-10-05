package config

import (
	"strings"
	"testing"
)

func TestCustomUploadRequiresConcreteBoundedCommand(t *testing.T) {
	valid := "version: 1\nsteps:\n  - kind: upload\n    target: custom\n    app_identifier: org.example.custom\n    file: dist/package.bin\n    argv: [bash, ci/upload.sh]\n    result_file: result.json\n"
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(valid, "    file: dist/package.bin\n", "", 1), strings.Replace(valid, "    app_identifier: org.example.custom\n", "", 1),
		strings.Replace(valid, "argv: [bash, ci/upload.sh]", "argv: [bash, \"bad\\0\"]", 1),
		strings.Replace(valid, "argv: [bash, ci/upload.sh]", "argv: [bash, '"+strings.Repeat("x", 4097)+"']", 1),
		valid + "    credentials: inline-secret\n",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Error("accepted unsafe custom upload")
		}
	}
}
