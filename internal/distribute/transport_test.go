package distribute

import (
	"context"
	"fmt"
	"mybuilds/internal/process"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func rubyTools(t *testing.T) string {
	t.Helper()
	gems := os.Getenv("MYBUILDS_TEST_GEMS")
	if gems == "" {
		gems = "/tmp/mybuilds-mvp.zKtK0e/publish-channel-gems"
	}
	if _, e := os.Stat(gems); e != nil {
		t.Skip("真实Ruby故障门需要已安装锁版本工具目录")
	}
	root := t.TempDir()
	for _, name := range toolNames {
		data, e := toolSource.ReadFile("fastlane/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if os.WriteFile(filepath.Join(root, name), data, 0600) != nil {
			t.Fatal(name)
		}
	}
	if os.Symlink(gems, filepath.Join(root, "gems")) != nil {
		t.Fatal("部署gems")
	}
	return root
}
func TestActualLockedRubyMutationSingleRequest(t *testing.T) {
	toolDir := rubyTools(t)
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		switch {
		case strings.Contains(r.URL.Path, "positive"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"owned"}`))
		case strings.Contains(r.URL.Path, "content-start") && r.Method == "POST":
			mode := ""
			for _, part := range strings.Split(r.URL.Path, "/") {
				if strings.HasPrefix(part, "content-start-") {
					mode = strings.TrimPrefix(part, "content-start-")
				}
			}
			w.Header().Set("Location", "http://"+r.Host+"/content-"+mode)
			w.Header().Set("X-Goog-Upload-URL", "http://"+r.Host+"/content-"+mode)
			w.Header().Set("X-Goog-Upload-Status", "active")
			w.WriteHeader(200)
		case strings.Contains(r.URL.Path, "drop"):
			h := w.(http.Hijacker)
			c, _, _ := h.Hijack()
			c.Close()
		case strings.Contains(r.URL.Path, "redirect"):
			w.Header().Set("Location", serverURLPlaceholder)
			w.WriteHeader(307)
		default:
			code := 401
			for _, candidate := range []int{429, 500, 504} {
				if strings.Contains(r.URL.Path, fmt.Sprint(candidate)) {
					code = candidate
				}
			}
			w.WriteHeader(code)
			w.Write([]byte(`{"error":{"code":401,"message":"owned"}}`))
		}
	}))
	defer server.Close()
	p, e := newPrepared(context.Background(), toolDir, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	script := `require ARGV.shift
base=ARGV.shift
positive=MybuildsChannels::GooglePlay.service('own-fixed-token',base+'/').insert_edit('positive')
raise 'body_lost' unless positive.id=='owned'
['unauthorized','status429','status500','status504','drop','redirect'].each do |mode|
 s=MybuildsChannels::GooglePlay.service('own-fixed-nonrefresh-token',base+'/')
 begin;s.insert_edit(mode);rescue StandardError;end
 begin;s.commit_edit(mode,'owned');rescue StandardError;end
 track=Google::Apis::AndroidpublisherV3::Track.new(track:'internal',releases:[])
 begin;s.update_edit_track(mode,'owned','internal',track);rescue StandardError;end
 artifact=File.join(File.dirname(__FILE__),'own.aab');File.binwrite(artifact,'own-artifact-bytes')
 begin;s.upload_edit_bundle(mode,'owned',upload_source:artifact,content_type:'application/octet-stream');rescue StandardError;end
 begin;s.upload_edit_bundle('content-start-'+mode,'owned',upload_source:artifact,content_type:'application/octet-stream');rescue StandardError;end
 [:post,:patch].each do |method|
  begin;MybuildsChannels::Apple.request('own-fixed-token',method,'/'+mode+method.to_s,{'data'=>{}},base);rescue StandardError;end
 end
end
puts 'actual_library_done'
`
	file := filepath.Join(p.workDir, "fault.rb")
	os.WriteFile(file, []byte(script), 0600)
	env := p.environment("")
	env = append(env, "PATH=/opt/homebrew/opt/ruby/bin:"+os.Getenv("PATH"))
	var out, stderr limitedOutput
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result := process.Run(ctx, process.Command{Path: "/opt/homebrew/opt/ruby/bin/bundle", Args: []string{"exec", "ruby", file, filepath.Join(toolDir, "channel.rb"), server.URL}, Dir: toolDir, Env: env}, &out, &stderr)
	if result.ExitCode != 0 || result.CleanupFailed || result.Reason != "" {
		t.Fatalf("真实Ruby库失败 %s/%d safe-output-size=%d", result.Reason, result.ExitCode, stderr.data.Len())
	}
	mu.Lock()
	defer mu.Unlock()
	for path, n := range counts {
		if n != 1 {
			t.Fatalf("%s 重发 %d", path, n)
		}
	}
	if len(counts) != 49 {
		t.Fatalf("actual endpoints=%d output=%s", len(counts), out.data.String())
	}
	t.Log(fmt.Sprintf("真实Ruby库 %d 不可逆端点，每端点一次", len(counts)))
}

const serverURLPlaceholder = "http://127.0.0.1:1/forbidden_redirect"
