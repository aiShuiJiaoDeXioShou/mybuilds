package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"mybuilds/internal/config"
)

func TestEnqueueReportsFreezesDefinitionWithoutInventingEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("report-app"))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []*bool{nil, new(bool)} {
			key := "reports-default"
			if required != nil {
				key = "reports-optional"
			}
			input := enqueueInput(p, key)
			reports := &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"out/**/*.xml", "test/{{version}}.xml"}, Required: required}}
			input.Builds[0].Snapshot.Definition.Reports = reports
			result, err := s.Enqueue(testContext, input)
			if err != nil {
				t.Fatalf("严格报告配置未入队: %v", err)
			}
			view := result.Builds[0]
			if view.Status != "queued" || view.Reports != nil || view.ReportSealDigest != "" {
				t.Fatal("尚未运行的构建虚构报告或seal")
			}
			var row buildRecord
			if err = s.db.First(&row, "id = ?", view.ID).Error; err != nil {
				t.Fatal(err)
			}
			var snapshot BuildSnapshot
			if err = json.Unmarshal([]byte(row.SnapshotJSON), &snapshot); err != nil || !reflect.DeepEqual(snapshot.Definition.Reports, reports) {
				t.Fatalf("报告配置/required省略状态未冻结: %v", err)
			}
			if row.ReportRevision != 0 || row.ReportFinal || row.ReportsJSON != "" || row.ReportSealDigest != "" || row.ReportCheckedIndex != 0 {
				t.Fatal("入队回填本次执行证据")
			}
			replayed, err := s.Enqueue(testContext, input)
			if err != nil || !replayed.Replayed || replayed.Builds[0].ID != view.ID {
				t.Fatal("报告定义破坏幂等入队", err)
			}
		}
		current, err := s.GetProject(testContext, p.Name)
		if err != nil || current.NextNumber != p.NextNumber+2 {
			t.Fatalf("重复请求消耗额外编号: %v", err)
		}
	})
}

func TestEnqueueInvalidReportsRejectsWholeBatchWithoutNumber(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("invalid-report-app"))
		if err != nil {
			t.Fatal(err)
		}
		for _, paths := range [][]string{{"../outside.xml"}, {"/outside.xml"}, {"x\\report.xml"}, {strings.Repeat("a", 256)}, make([]string, 33)} {
			input := enqueueInput(p, "invalid-report")
			second := input.Builds[0]
			second.Name = "invalid-second"
			second.Snapshot.Definition.Reports = &config.Reports{JUnit: &config.JUnitReport{Paths: paths}}
			input.Builds = append(input.Builds, second)
			if _, err = s.Enqueue(testContext, input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("非法报告批次获准: %v", err)
			}
		}
		current, err := s.GetProject(testContext, p.Name)
		if err != nil || current.NextNumber != p.NextNumber {
			t.Fatal("校验失败消耗编号", err)
		}
		for _, model := range []any{&buildRecord{}, &batchRecord{}, &requestRecord{}} {
			var count int64
			if err = s.db.Model(model).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("未完整校验即保存部分批次", err)
			}
		}
	})
}
