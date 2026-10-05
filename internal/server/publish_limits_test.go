package server

import (
	"strings"
	"testing"

	"mybuilds/internal/store"
)

// 用可成功的原请求检验真实读取上限，避免无效JSON无论上限多少都失败。
func TestPublishHTTPActual64KiBBoundary(t *testing.T) {
	_, _, h, _, _ := executionHTTPFixture(t)
	in := store.BindApplicationInput{NodeID: "worker", Store: "google_play", AppIdentifier: "com.example.boundedpublish", CredentialRef: "${PLAY_FILE}", AllowedTracks: []string{"internal"}, UploadCertificateSHA256: strings.Repeat("a", 64)}
	body := encodeMessage(t, in)
	for _, size := range []int{65537, 65536} {
		padded := body + strings.Repeat(" ", size-len(body))
		code, _ := request(t, h, "POST", "/api/projects/app/applications", adminToken, padded)
		want := 400
		if size == 65536 {
			want = 202
		}
		if code != want {
			t.Fatalf("发布请求%d字节: 得到%d，要求%d", size, code, want)
		}
	}
}
