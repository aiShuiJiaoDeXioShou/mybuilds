package mobile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"mybuilds/internal/process"
)

// IOSResourceIdentity 保存真实文件身份，不保存秘密或把路径当所有权证明。
type IOSResourceIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Mode   uint32 `json:"mode"`
}

// IOSResourceOwnership 只用于私有journal，不能放入公共构建或日志输出。
type IOSResourceOwnership struct {
	Version            int                  `json:"version"`
	Token              string               `json:"token"`
	Workspace          string               `json:"workspace"`
	Output             string               `json:"output"`
	Temporary          string               `json:"temporary"`
	Keychain           string               `json:"keychain"`
	Profile            string               `json:"profile"`
	OutputIdentity     IOSResourceIdentity  `json:"output_identity"`
	TemporaryIdentity  IOSResourceIdentity  `json:"temporary_identity"`
	KeychainIdentity   *IOSResourceIdentity `json:"keychain_identity,omitempty"`
	KeychainDBIdentity *IOSResourceIdentity `json:"keychain_db_identity,omitempty"`
	ProfileIdentity    *IOSResourceIdentity `json:"profile_identity,omitempty"`
	ProfileSHA256      string               `json:"profile_sha256,omitempty"`
	Preparing          bool                 `json:"preparing,omitempty"`
	Prepared           bool                 `json:"prepared,omitempty"`
	Closed             bool                 `json:"closed,omitempty"`
}

var iosOwnershipToken = regexp.MustCompile(`^[a-f0-9]{32}$`)

// PlanIOSResources 只创建空的自有目录，不读取材料或执行原生导入。
func PlanIOSResources(ctx context.Context, options IOSSigningOptions) (resources *IOSResources, err error) {
	if !iosNativeAvailable() {
		return nil, errors.New("ios_signing_unsupported")
	}
	if err = validateIOSOptions(options); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, errors.New("ios_prepare_cancelled")
	}
	workspace, e := filepath.Abs(options.Workspace)
	if e == nil {
		workspace, e = filepath.EvalSymlinks(workspace)
	}
	if e != nil {
		return nil, errors.New("ios_workspace_invalid")
	}
	if path.IsAbs(options.OutputDir) || path.Clean(options.OutputDir) != options.OutputDir || options.OutputDir == "." || strings.ContainsAny(options.OutputDir, "\\:") || strings.Contains(options.OutputDir, "..") || options.OutputDir == "" || strings.IndexFunc(options.OutputDir, unicode.IsControl) >= 0 {
		return nil, errors.New("ios_output_invalid")
	}
	output := filepath.Join(workspace, filepath.FromSlash(options.OutputDir))
	parent, e := filepath.EvalSymlinks(filepath.Dir(output))
	if e != nil || parent != filepath.Dir(output) {
		return nil, errors.New("ios_output_invalid")
	}
	token, e := iosRandomToken()
	if e != nil {
		return nil, e
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return nil, errors.New("ios_profile_install_failed")
	}
	resources = &IOSResources{workspace: workspace, output: output, token: token, env: map[string]string{}, keychainInfo: map[string]os.FileInfo{}}
	resources.profile = filepath.Join(home, "Library", "Developer", "Xcode", "UserData", "Provisioning Profiles", "mybuilds-"+token+".mobileprovision")
	if e = os.Mkdir(output, 0700); e != nil {
		return nil, errors.New("ios_output_unavailable")
	}
	defer func() {
		if err != nil {
			closeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			closeErr := resources.Close(closeCtx)
			stop()
			if closeErr != nil {
				err = errors.Join(err, ErrIOSCleanup)
			}
			resources = nil
		}
	}()
	resources.outputInfo, e = os.Lstat(output)
	if e != nil {
		return resources, errors.New("ios_output_unavailable")
	}
	resources.temporary, e = process.TemporaryDirectory(workspace, "mybuilds-ios-")
	if e != nil {
		return resources, errors.New("ios_temporary_unavailable")
	}
	resources.temporaryInfo, e = os.Lstat(resources.temporary)
	if e != nil {
		return resources, errors.New("ios_temporary_unavailable")
	}
	resources.keychain = filepath.Join(resources.temporary, "signing.keychain")
	if ctx.Err() != nil {
		return resources, errors.New("ios_prepare_cancelled")
	}
	return resources, nil
}

// Ownership 返回独立的可序列化证据；调用者在Prepare前必须持久化准备intent。
func (resources *IOSResources) Ownership() IOSResourceOwnership {
	resources.mu.Lock()
	defer resources.mu.Unlock()
	out := IOSResourceOwnership{Version: 1, Token: resources.token, Workspace: resources.workspace, Output: resources.output, Temporary: resources.temporary, Keychain: resources.keychain, Profile: resources.profile, Preparing: resources.preparing, Prepared: resources.prepared, Closed: resources.closed}
	if resources.restoredOwnership != nil {
		out.OutputIdentity = resources.restoredOwnership.OutputIdentity
		out.TemporaryIdentity = resources.restoredOwnership.TemporaryIdentity
		out.KeychainIdentity = resources.restoredOwnership.KeychainIdentity
		out.KeychainDBIdentity = resources.restoredOwnership.KeychainDBIdentity
		out.ProfileIdentity = resources.restoredOwnership.ProfileIdentity
		out.ProfileSHA256 = resources.restoredOwnership.ProfileSHA256
	}
	if resources.outputInfo != nil {
		out.OutputIdentity, _ = iosIdentity(resources.outputInfo)
	}
	if resources.temporaryInfo != nil {
		out.TemporaryIdentity, _ = iosIdentity(resources.temporaryInfo)
	}
	for name, info := range resources.keychainInfo {
		v, e := iosIdentity(info)
		if e == nil {
			if name == resources.keychain {
				out.KeychainIdentity = &v
			} else if name == resources.keychain+"-db" {
				out.KeychainDBIdentity = &v
			}
		}
	}
	if v, e := iosIdentity(resources.profileInfo); e == nil {
		out.ProfileIdentity = &v
		out.ProfileSHA256 = fmtIOSHash(resources.profileHash)
	}
	for _, identity := range []**IOSResourceIdentity{&out.KeychainIdentity, &out.KeychainDBIdentity, &out.ProfileIdentity} {
		if *identity != nil {
			value := **identity
			*identity = &value
		}
	}
	return out
}

// RestoreIOSResources 不重新导入材料，现有叶没有原身份时立即拒绝接管。
func RestoreIOSResources(ctx context.Context, own IOSResourceOwnership) (*IOSResources, error) {
	if ctx.Err() != nil {
		return nil, ErrIOSCleanup
	}
	if !ValidIOSResourceOwnership(own) || own.Version != 1 || !iosOwnershipToken.MatchString(own.Token) || !filepath.IsAbs(own.Workspace) || !filepath.IsAbs(own.Output) || !filepath.IsAbs(own.Temporary) || own.Keychain != filepath.Join(own.Temporary, "signing.keychain") {
		return nil, ErrIOSCleanup
	}
	workspace, e := filepath.EvalSymlinks(own.Workspace)
	if e != nil || workspace != own.Workspace {
		return nil, ErrIOSCleanup
	}
	rel, e := filepath.Rel(own.Workspace, own.Output)
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrIOSCleanup
	}
	rel, e = filepath.Rel(own.Workspace, own.Temporary)
	if e != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) || !strings.HasPrefix(filepath.Base(own.Temporary), "mybuilds-ios-") {
		return nil, ErrIOSCleanup
	}
	home, e := os.UserHomeDir()
	if e != nil || own.Profile != filepath.Join(home, "Library", "Developer", "Xcode", "UserData", "Provisioning Profiles", "mybuilds-"+own.Token+".mobileprovision") {
		return nil, ErrIOSCleanup
	}
	resources := &IOSResources{workspace: own.Workspace, output: own.Output, temporary: own.Temporary, keychain: own.Keychain, profile: own.Profile, token: own.Token, preparing: own.Preparing, prepared: own.Prepared, closed: own.Closed, keychainInfo: map[string]os.FileInfo{}, env: map[string]string{}}
	saved := own
	for _, identity := range []**IOSResourceIdentity{&saved.KeychainIdentity, &saved.KeychainDBIdentity, &saved.ProfileIdentity} {
		if *identity != nil {
			value := **identity
			*identity = &value
		}
	}
	resources.restoredOwnership = &saved
	resources.outputInfo, e = restoreIOSIdentity(own.Output, &own.OutputIdentity, true)
	if e != nil {
		return nil, e
	}
	resources.temporaryInfo, e = restoreIOSIdentity(own.Temporary, &own.TemporaryIdentity, true)
	if e != nil {
		return nil, e
	}
	for _, leaf := range []struct {
		path     string
		identity *IOSResourceIdentity
	}{{own.Keychain, own.KeychainIdentity}, {own.Keychain + "-db", own.KeychainDBIdentity}} {
		info, e := restoreIOSIdentity(leaf.path, leaf.identity, false)
		if e != nil {
			return nil, e
		}
		if info != nil {
			resources.keychainInfo[leaf.path] = info
		}
	}
	resources.profileInfo, e = restoreIOSIdentity(own.Profile, own.ProfileIdentity, false)
	if e != nil {
		return nil, e
	}
	if own.ProfileIdentity != nil {
		decoded, e := parseIOSHash(own.ProfileSHA256)
		if e != nil {
			return nil, ErrIOSCleanup
		}
		resources.profileHash = decoded
	}
	if own.Prepared && (own.Preparing || (own.KeychainIdentity == nil && own.KeychainDBIdentity == nil) || own.ProfileIdentity == nil) {
		return nil, ErrIOSCleanup
	}
	return resources, nil
}

func restoreIOSIdentity(filename string, expected *IOSResourceIdentity, directory bool) (os.FileInfo, error) {
	if expected != nil && (expected.Inode == 0 || os.FileMode(expected.Mode).IsDir() != directory || (!directory && !os.FileMode(expected.Mode).IsRegular())) {
		return nil, ErrIOSCleanup
	}
	info, e := os.Lstat(filename)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil || expected == nil {
		return nil, ErrIOSCleanup
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(filename))
	if e != nil || parent != filepath.Dir(filename) {
		return nil, ErrIOSCleanup
	}
	actual, e := iosIdentity(info)
	if e != nil || actual != *expected {
		return nil, ErrIOSCleanup
	}
	return info, nil
}

func fmtIOSHash(value [32]byte) string { return hex.EncodeToString(value[:]) }
func parseIOSHash(value string) ([32]byte, error) {
	var hash [32]byte
	data, err := hex.DecodeString(value)
	if err != nil || len(data) != 32 {
		return hash, ErrIOSCleanup
	}
	copy(hash[:], data)
	return hash, nil
}

// ValidIOSResourceOwnership 对私有checkpoint作纯形状校验，不把字段当作文件授权。
func ValidIOSResourceOwnership(own IOSResourceOwnership) bool {
	if own.Version != 1 || !iosOwnershipToken.MatchString(own.Token) || own.Closed && own.Preparing || own.Prepared && own.Preparing {
		return false
	}
	for _, filename := range []string{own.Workspace, own.Output, own.Temporary, own.Keychain, own.Profile} {
		if !filepath.IsAbs(filename) || filepath.Clean(filename) != filename || len(filename) > 4096 || strings.IndexFunc(filename, unicode.IsControl) >= 0 {
			return false
		}
	}
	rel, err := filepath.Rel(own.Workspace, own.Output)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	rel, err = filepath.Rel(own.Workspace, own.Temporary)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !strings.HasPrefix(filepath.Base(own.Temporary), "mybuilds-ios-") || own.Keychain != filepath.Join(own.Temporary, "signing.keychain") || filepath.Base(own.Profile) != "mybuilds-"+own.Token+".mobileprovision" {
		return false
	}
	for _, identity := range []IOSResourceIdentity{own.OutputIdentity, own.TemporaryIdentity} {
		if identity.Inode == 0 || !os.FileMode(identity.Mode).IsDir() {
			return false
		}
	}
	for _, identity := range []*IOSResourceIdentity{own.KeychainIdentity, own.KeychainDBIdentity, own.ProfileIdentity} {
		if identity != nil && (identity.Inode == 0 || !os.FileMode(identity.Mode).IsRegular()) {
			return false
		}
	}
	if own.ProfileIdentity != nil {
		if _, err := parseIOSHash(own.ProfileSHA256); err != nil {
			return false
		}
	} else if own.ProfileSHA256 != "" {
		return false
	}
	return !own.Prepared || (own.KeychainIdentity != nil || own.KeychainDBIdentity != nil) && own.ProfileIdentity != nil
}

// IOSResourceDigest 绑定私有关闭checkpoint，网络只传摘要，不传路径或原材料。
func IOSResourceDigest(own IOSResourceOwnership) (string, error) {
	if !ValidIOSResourceOwnership(own) || !own.Closed {
		return "", ErrIOSCleanup
	}
	data, err := json.Marshal(own)
	if err != nil {
		return "", ErrIOSCleanup
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
