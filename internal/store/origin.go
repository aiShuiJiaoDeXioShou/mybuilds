package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/config"
	"slices"
)

// PipelineOrigin仅存固定来源证据，不存管理员本地文件路径或方案字节。
type PipelineOrigin struct {
	Mode             string `json:"mode"`
	Kind             string `json:"kind"`
	SHA              string `json:"sha"`
	File             string `json:"file,omitempty"`
	Profile          string `json:"profile,omitempty"`
	Template         string `json:"template,omitempty"`
	ContentDigest    string `json:"content_digest"`
	DefinitionDigest string `json:"definition_digest"`
}
type PipelineOriginView = PipelineOrigin

func DefinitionDigest(definition config.Build) string {
	definition.Notifications = nil
	data, _ := json.Marshal(definition)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func validateOrigin(origin *PipelineOrigin, definition config.Build) bool {
	if origin == nil {
		return true
	}
	if !slices.Contains([]string{"auto", "repo", "profile"}, origin.Mode) || !validHex(origin.SHA, 40, 64) || !validHex(origin.ContentDigest, 64) || origin.DefinitionDigest != DefinitionDigest(definition) {
		return false
	}
	switch origin.Kind {
	case "repo":
		return origin.Mode != "profile" && sourceFileValid(origin.File) && origin.Profile == "" && origin.Template == ""
	case "profile":
		return origin.Mode != "repo" && origin.File == "" && validName(origin.Profile) && len(origin.Profile) <= 64 && (origin.Template == "" || slices.Contains([]string{"native-android", "native-ios", "flutter-android", "flutter-ios"}, origin.Template))
	}
	return false
}
func originBatchMatches(origin *PipelineOrigin, sha, mode, file string) bool {
	return origin == nil || origin.SHA == sha && origin.Mode == mode && (origin.Kind != "repo" || origin.File == file)
}
