package distribute

import (
	"context"
	"encoding/json"
	"mybuilds/internal/process"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestActualAppleQueryConfirmsOnlyOriginalGETRelationships(t *testing.T) {
	tools := rubyTools(t)
	var reads, writes atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes.Add(1)
			w.WriteHeader(405)
			return
		}
		reads.Add(1)
		var body any
		switch r.URL.Path {
		case "/v1/appStoreVersions/version-owned":
			body = map[string]any{"data": map[string]any{"id": "version-owned", "attributes": map[string]any{"releaseType": "MANUAL"}, "relationships": map[string]any{"app": map[string]any{"data": map[string]any{"id": "app-owned"}}, "build": map[string]any{"data": map[string]any{"id": "build-owned"}}}}}
		case "/v1/reviewSubmissions/review-owned":
			body = map[string]any{"data": map[string]any{"id": "review-owned", "attributes": map[string]any{"state": "WAITING_FOR_REVIEW", "submittedDate": "2026-10-05T00:00:00Z"}, "relationships": map[string]any{"app": map[string]any{"data": map[string]any{"id": "app-owned"}}}}}
		case "/v1/reviewSubmissions/review-owned/items":
			body = map[string]any{"data": []any{map[string]any{"id": "item-owned", "relationships": map[string]any{"appStoreVersion": map[string]any{"data": map[string]any{"id": "version-owned"}}}}}}
		case "/v1/builds":
			body = map[string]any{"data": []any{map[string]any{"id": "unrelated", "attributes": map[string]any{"processingState": "VALID"}}}}
		default:
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
	defer endpoint.Close()
	p, e := newPrepared(context.Background(), tools, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	script := `require ARGV.shift
base=ARGV.shift
a={'action'=>'submit_review','request_sha256'=>'a'*64,'app_store_version_id'=>'version-owned','build_id'=>'build-owned','review_submission_id'=>'review-owned','review_item_id'=>'item-owned'}
job={'query'=>{'apple_authorization'=>a,'version_code'=>101}}
out=MybuildsChannels::Apple.query('own-token',job,'app-owned',base)
raise 'no_original_confirmation' unless out['status']=='confirmed'&&out['matches'].length==1&&out['matches'][0]['apple']['action_confirmed']&&out['matches'][0]['apple']['review_item_id']=='item-owned'
a['build_id']='foreign-build'
begin;MybuildsChannels::Apple.query('own-token',job,'app-owned',base);raise 'wrong_build_accepted';rescue StandardError=>e;raise if e.message=='wrong_build_accepted';end
a['build_id']='build-owned';a['review_item_id']='foreign-item'
begin;MybuildsChannels::Apple.query('own-token',job,'app-owned',base);raise 'wrong_item_accepted';rescue StandardError=>e;raise if e.message=='wrong_item_accepted';end
a['action']='upload_binary'
out=MybuildsChannels::Apple.query('own-token',job,'app-owned',base)
raise 'upload_promoted_by_version' unless out['reason']=='observation_insufficient'&&!out['matches'][0]['apple']['action_confirmed']
puts 'GET-only-original-relationship-confirmation'
`
	file := filepath.Join(p.workDir, "query.rb")
	if e = os.WriteFile(file, []byte(script), 0600); e != nil {
		t.Fatal(e)
	}
	var out, stderr limitedOutput
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := process.Run(ctx, process.Command{Path: "/opt/homebrew/opt/ruby/bin/bundle", Args: []string{"exec", "ruby", file, filepath.Join(tools, "channel.rb"), endpoint.URL}, Dir: tools, Env: append(p.environment(""), "PATH=/opt/homebrew/opt/ruby/bin:"+os.Getenv("PATH"))}, &out, &stderr)
	if run.ExitCode != 0 || run.Reason != "" || run.CleanupFailed {
		t.Fatalf("actual query failed: %s/%d stderr bytes %d", run.Reason, run.ExitCode, stderr.data.Len())
	}
	if reads.Load() != 10 || writes.Load() != 0 {
		t.Fatal("unexpected endpoint count", reads.Load(), writes.Load())
	}
}
