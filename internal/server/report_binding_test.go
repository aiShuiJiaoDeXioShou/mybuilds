package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func TestReportHTTPCurrentRevisionAndRunSourceFence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*protocol.ArtifactDeclaration)
	}{
		{"旧revision", func(d *protocol.ArtifactDeclaration) { d.ReportRevision-- }},
		{"旧epoch", func(d *protocol.ArtifactDeclaration) { d.Ref.Epoch++ }},
		{"错误run index", func(d *protocol.ArtifactDeclaration) { d.Index = 2 }},
		{"错误run name", func(d *protocol.ArtifactDeclaration) { d.Step = "other-run" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
			_, st, api, token, d, evidence := reportHTTPFixture(t, data, nil)
			wrong := d
			tc.change(&wrong)
			code, body := uploadArtifactHTTP(t, api, token, wrong, data)
			if code != 409 {
				t.Fatal("旧归属未拒绝", code, string(body))
			}
			actor, err := st.AuthenticateNode(context.Background(), token)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = st.FindNodeArtifact(context.Background(), actor, d.Ref, d.ID); err == nil {
				t.Fatal("错误归属得到当前回执")
			}
			sealReportHTTP(t, api, token, d, evidence, 409)
			code, body = uploadArtifactHTTP(t, api, token, d, data)
			if code != 200 {
				t.Fatal("错误请求破坏后续合法上传", code, string(body))
			}
			sealReportHTTP(t, api, token, d, evidence, 200)
		})
	}
}

func TestReportHTTPStrictDeclarationCannotInjectVerifiedResult(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	_, st, api, token, d, _ := reportHTTPFixture(t, data, nil)
	canonical := encodeMessage(t, d)
	for _, raw := range []string{
		canonical[:len(canonical)-1] + `,"verified_junit":{"counts":{"tests":1}}}`,
		canonical[:len(canonical)-1] + `,"purpose":"junit"}`,
		strings.Replace(canonical, `"purpose":"junit"`, `"purpose":null`, 1),
		strings.Replace(canonical, `"report_key"`, `"Report_key"`, 1),
	} {
		r, err := http.NewRequest("PUT", api.URL+"/api/agent/artifacts/"+d.ID, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(raw)))
		response, err := api.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("非法HTTP声明被接受", response.StatusCode)
		}
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	files, err := st.ListArtifacts(context.Background(), actor, d.Ref.BuildID, store.Page{Limit: 100})
	if err != nil || len(files) != 0 {
		t.Fatal("伪摘要得到用户证据", files, err)
	}
}
