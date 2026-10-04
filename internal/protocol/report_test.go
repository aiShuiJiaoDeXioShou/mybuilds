package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 新消息必须往返保留报告证据，同时原消息不产生假报告字段。
func TestReportWireRoundTrip(t *testing.T) {
	input := `{"kind":"reports_checked","reports":{"revision":1,"sealed":false,"outcome":"pending","reason":"","required":true,"counts":{"tests":0,"failures":0,"errors":0,"skipped":0,"duration_ns":0},"diagnostics":[],"files":[]},"report_manifest":{"seal_digest":"digest","ids":[]}}`
	var progress ExecutionProgress
	if err := json.Unmarshal([]byte(input), &progress); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(progress)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected map[string]json.RawMessage
	json.Unmarshal(data, &actual)
	json.Unmarshal([]byte(input), &expected)
	for _, field := range []string{"reports", "report_manifest"} {
		var got, want any
		json.Unmarshal(actual[field], &got)
		json.Unmarshal(expected[field], &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s 丢失或改变报告证据: %s", field, actual[field])
		}
	}
	var declaration ArtifactDeclaration
	json.Unmarshal([]byte(`{"purpose":"junit","report_revision":2,"report_key":"key"}`), &declaration)
	data, _ = json.Marshal(declaration)
	var fields map[string]any
	json.Unmarshal(data, &fields)
	if fields["purpose"] != "junit" || fields["report_revision"] != float64(2) || fields["report_key"] != "key" {
		t.Fatalf("原文件协议未保留JUnit用途: %s", data)
	}
}

func TestReportFieldsDoNotChangeLegacyWire(t *testing.T) {
	for _, value := range []any{ExecutionProgress{}, ArtifactDeclaration{}, ArtifactView{}} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{`"reports"`, `"report_manifest"`, `"purpose"`, `"report_revision"`, `"report_key"`} {
			if strings.Contains(string(data), field) {
				t.Fatalf("旧消息出现新字段 %s: %s", field, data)
			}
		}
	}
}

func TestReportEmptyArraysAndPrivatePaths(t *testing.T) {
	progress := reflect.ValueOf(&ExecutionProgress{}).Elem()
	for _, name := range []string{"Reports", "ReportManifest"} {
		field := progress.FieldByName(name)
		if !field.IsValid() || field.Kind() != reflect.Pointer {
			t.Fatalf("缺少具体报告字段 %s", name)
		}
		field.Set(reflect.New(field.Type().Elem()))
	}
	local := progress.FieldByName("LocalReports")
	if !local.IsValid() || local.Kind() != reflect.Slice {
		t.Fatal("缺少私有报告快照通道")
	}
	item := reflect.New(local.Type().Elem()).Elem()
	item.FieldByName("SnapshotPath").SetString("/private/report-secret.xml")
	local.Set(reflect.Append(local, item))
	data, err := json.Marshal(progress.Interface())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "report-secret") || strings.Contains(string(data), "LocalReports") {
		t.Fatalf("私有路径进入网络消息: %s", data)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(data, &fields)
	for _, check := range []struct{ parent, child string }{{"reports", "diagnostics"}, {"reports", "files"}, {"report_manifest", "ids"}} {
		var nested map[string]json.RawMessage
		json.Unmarshal(fields[check.parent], &nested)
		if string(nested[check.child]) != "[]" {
			t.Errorf("%s.%s 空集合必须为[]: %s", check.parent, check.child, nested[check.child])
		}
	}
}
