package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublishBuildViewLinksActualIntentOnly(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := publishFixture(t, s)
		before, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(before)
		if strings.Contains(string(data), `"publish_ids"`) {
			t.Fatal("未授意图虚构关联")
		}
		if _, err = s.AuthorizePublish(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		after, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil {
			t.Fatal(err)
		}
		data, _ = json.Marshal(after)
		var view map[string]json.RawMessage
		if err = json.Unmarshal(data, &view); err != nil {
			t.Fatal(err)
		}
		var ids []string
		if err = json.Unmarshal(view["publish_ids"], &ids); err != nil || len(ids) != 1 || ids[0] != in.IntentID {
			t.Fatal("缺少真实意图关联", err)
		}
		if strings.Contains(string(data), "PLAY_JSON") || strings.Contains(string(data), "authorization_digest") {
			t.Fatal("安全构建视图泄漏私有授权")
		}
	})
}
