package distribute

import (
	"context"
	"encoding/json"
	"fmt"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActualAppleNextActionReadOnlyChain(t *testing.T) {
	tools := rubyTools(t)
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if r.Method != "GET" {
			t.Errorf("只读规划产生写请求 %s", r.Method)
			w.WriteHeader(405)
			return
		}
		mode := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer own-")
		build := map[string]any{"type": "builds", "id": "build-owned", "attributes": map[string]any{"version": "101", "processingState": "VALID"}, "relationships": map[string]any{"preReleaseVersion": map[string]any{"data": map[string]any{"id": "pre-owned"}}}}
		if mode == "invalid" {
			build["attributes"].(map[string]any)["processingState"] = "INVALID"
		}
		version := map[string]any{"id": "version-owned", "attributes": map[string]any{"versionString": "1.2", "appStoreState": "PREPARE_FOR_SUBMISSION", "releaseType": "MANUAL"}, "relationships": map[string]any{"build": map[string]any{"data": map[string]any{"id": "build-owned"}}}}
		if mode == "select" {
			version["relationships"].(map[string]any)["build"] = map[string]any{"data": nil}
		}
		if mode == "policy" {
			version["attributes"].(map[string]any)["releaseType"] = "AFTER_APPROVAL"
		}
		var out any
		switch r.URL.Path {
		case "/v1/builds":
			out = map[string]any{"data": []any{build}, "included": []any{map[string]any{"id": "pre-owned", "type": "preReleaseVersions", "attributes": map[string]any{"version": "1.2", "platform": "IOS"}}}}
		case "/v1/apps/app-owned/appStoreVersions":
			out = map[string]any{"data": []any{version}}
		case "/v1/reviewSubmissions":
			rows := []any{}
			if mode == "foreign" {
				rows = append(rows, map[string]any{"attributes": map[string]any{"state": "READY_FOR_REVIEW"}})
			}
			out = map[string]any{"data": rows}
		case "/v1/reviewSubmissions/review-owned":
			out = map[string]any{"data": map[string]any{"id": "review-owned", "attributes": map[string]any{"state": "READY_FOR_REVIEW"}, "relationships": map[string]any{"app": map[string]any{"data": map[string]any{"id": "app-owned"}}}}}
		case "/v1/reviewSubmissions/review-owned/items":
			rows := []any{}
			if mode == "submit" {
				rows = append(rows, map[string]any{"id": "item-owned", "relationships": map[string]any{"appStoreVersion": map[string]any{"data": map[string]any{"id": "version-owned"}}}})
			}
			out = map[string]any{"data": rows}
		default:
			t.Errorf("未知GET %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	p, e := newPrepared(context.Background(), tools, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	script := `require ARGV.shift
base=ARGV.shift
job={'number'=>101,'version_name'=>'1.2','submit'=>true,'automatic'=>false,'previous'=>[{'mutation_stage'=>'upload_binary'}]}
{'select'=>'select_build','policy'=>'set_release_policy','create'=>'create_review','item'=>'add_review_item','submit'=>'submit_review'}.each do |mode,expected|
 current=Marshal.load(Marshal.dump(job))
 if ['item','submit'].include?(mode)
 current['previous']<<{'mutation_stage'=>'create_review','remote'=>{'apple'=>{'review_submission_id'=>'review-owned'}}}
 end
 if mode=='submit'
 current['previous']<<{'mutation_stage'=>'add_review_item','remote'=>{'apple'=>{'review_item_id'=>'item-owned'}}}
 end
 action=MybuildsChannels::Apple.next_action('own-'+mode,current,'app-owned',base)
 raise 'next_mismatch' unless action['action']==expected
 method,path,body=MybuildsChannels::Apple.body(expected,action.merge('app_id'=>'app-owned'))
 puts expected+':'+Digest::SHA256.hexdigest(JSON.generate(MybuildsChannels::Apple.canonical(body)))
end
['foreign','invalid'].each do |mode|
 begin
 MybuildsChannels::Apple.next_action('own-'+mode,job,'app-owned',base)
 raise 'unsafe_accepted'
 rescue StandardError=>e
 raise if e.message=='unsafe_accepted'
 end
end
job['previous']<<{'mutation_stage'=>'submit_review'}
raise 'finished_chain_replayed' unless MybuildsChannels::Apple.next_action('own-done',job,'app-owned',base).nil?
executor=FastlaneCore::AltoolTransporterExecutor.new
command=executor.build_upload_command(nil,nil,'/owned path/app.ipa',{platform:'ios',api_key:{key_id:'OWNKEY',issuer_id:'own-issuer',key_dir:'/owned-key'}})
raise 'tool_boundary' unless command.include?('xcrun altool --upload-app')&&!command.include?('-jwt')
parent_group=Process.getpgrp
IO.popen(['/bin/sleep','0.05'],'r'){|io|raise 'escaped_group' unless Process.getpgid(io.pid)==parent_group;io.read}
puts 'read_only_chain_done'
`
	file := filepath.Join(p.workDir, "next.rb")
	os.WriteFile(file, []byte(script), 0600)
	env := append(p.environment(""), "PATH=/opt/homebrew/opt/ruby/bin:"+os.Getenv("PATH"))
	var out, stderr limitedOutput
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result := process.Run(ctx, process.Command{Path: "/opt/homebrew/opt/ruby/bin/bundle", Args: []string{"exec", "ruby", file, filepath.Join(tools, "channel.rb"), server.URL}, Dir: tools, Env: env}, &out, &stderr)
	if result.ExitCode != 0 || result.CleanupFailed || result.Reason != "" {
		t.Fatalf("真实只读链失败 %s/%d safe-output=%d", result.Reason, result.ExitCode, stderr.data.Len())
	}
	apple := &PreparedApple{options: AppleOptions{AppIdentifier: "com.owned.app", VersionName: "1.2", Number: 101}, AppID: "app-owned"}
	for _, action := range []string{"select_build", "set_release_policy", "create_review", "add_review_item", "submit_review"} {
		digest, e := apple.RequestSHA256(protocol.ApplePublishAuthorization{Action: action, AppStoreVersionID: "version-owned", BuildID: "build-owned", ReviewSubmissionID: "review-owned", ReviewItemID: "item-owned", SubmitForReview: true})
		if e != nil || !strings.Contains(out.data.String(), action+":"+digest) {
			t.Fatalf("授权正文摘要不一致 %s", action)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	t.Log(fmt.Sprintf("实际GET-only链 %d请求，5动作规划及2拒绝，无写/重放", requests))
}
