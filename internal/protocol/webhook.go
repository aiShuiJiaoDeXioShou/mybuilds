package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type WebhookEvent struct {
	Provider      string `json:"provider"`
	Kind          string `json:"kind"`
	DeliveryID    string `json:"delivery_id"`
	RepositoryKey string `json:"repository_key"`
	Branch        string `json:"branch"`
	Before        string `json:"before"`
	After         string `json:"after"`
	BodyDigest    string `json:"body_digest"`
	ReceiptDigest string `json:"receipt_digest"`
	Reason        string `json:"reason"`
}
type ChangeFacts struct {
	Mode            string   `json:"mode"`
	Reason          string   `json:"reason,omitempty"`
	BaselineBuildID string   `json:"baseline_build_id,omitempty"`
	BaselineSHA     string   `json:"baseline_sha,omitempty"`
	TargetSHA       string   `json:"target_sha"`
	Paths           []string `json:"paths"`
	Digest          string   `json:"digest"`
}

var changeOID = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

func ChangesDigest(paths []string) string {
	bytes, _ := json.Marshal(paths)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}
func WebhookEventDigest(event WebhookEvent) string {
	event.ReceiptDigest = ""
	bytes, _ := json.Marshal(event)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}
func ValidChangePath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || path.IsAbs(value) || strings.ContainsAny(value, "\\:") || strings.IndexFunc(value, unicode.IsControl) >= 0 || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "." || part == "" || len(part) > 255 {
			return false
		}
	}
	return true
}

// ValidateChanges只校验已冻结事实，既不Git也不猜缺失基线。
func ValidateChanges(facts *ChangeFacts, target string) bool {
	if facts == nil {
		return true
	}
	if facts.TargetSHA != target || !changeOID.MatchString(target) || facts.Paths == nil || len(facts.Paths) > 100000 || facts.Digest != ChangesDigest(facts.Paths) {
		return false
	}
	if facts.Mode == "full" {
		return len(facts.Paths) == 0 && (facts.Reason == "baseline_missing" || facts.Reason == "baseline_unavailable") && (facts.BaselineSHA == "" || changeOID.MatchString(facts.BaselineSHA))
	}
	if facts.Mode != "diff" || facts.Reason != "" || !changeOID.MatchString(facts.BaselineSHA) || !slices.IsSorted(facts.Paths) {
		return false
	}
	size := 0
	previous := ""
	for _, value := range facts.Paths {
		size += len(value)
		if size > 8<<20 || !ValidChangePath(value) || value == previous {
			return false
		}
		previous = value
	}
	return true
}
