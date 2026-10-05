package mobile

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrIOSCleanup 让唯一pipeline所有者区分未确认清理，保留原失败并停止批次。
var ErrIOSCleanup = errors.New("ios_cleanup_failed")

const iosResourceBudget = 15 * time.Second

type iosHelperRequest struct {
	Action   string            `json:"action"`
	Options  IOSSigningOptions `json:"options"`
	Keychain string            `json:"keychain,omitempty"`
}
type iosHelperResponse struct{ Reason, Identity, UUID, Team, ProfileDigest string }
type iosMaterial struct {
	identity, uuid, team string
	profile, p12         []byte
}

type IOSResources struct {
	team                                            string
	keychainInfo                                    map[string]os.FileInfo
	mu                                              sync.Mutex
	closed, preparing, prepared                     bool
	token                                           string
	restoredOwnership                               *IOSResourceOwnership
	workspace, temporary, output, keychain, profile string
	temporaryInfo, outputInfo, profileInfo          os.FileInfo
	profileHash                                     [32]byte
	env                                             map[string]string
}

func ValidateIOSSigning(ctx context.Context, options IOSSigningOptions) error {
	if !iosNativeAvailable() {
		return errors.New("ios_signing_unsupported")
	}
	if err := validateIOSOptions(options); err != nil {
		return err
	}
	_, err := invokeIOSHelper(ctx, iosHelperRequest{Action: "inspect", Options: options})
	return err
}

func validateIOSOptions(options IOSSigningOptions) error {
	if !iosBundleID.MatchString(options.BundleID) || len(options.BundleID) > 255 || !validIOSMethod(options.ExportMethod) {
		return errors.New("ios_signing_options_invalid")
	}
	if options.P12File == "" || options.ProfileFile == "" || len(options.Password) > 16384 || strings.ContainsRune(options.Password, 0) {
		return errors.New("ios_signing_options_invalid")
	}
	return nil
}

func invokeIOSHelper(ctx context.Context, request iosHelperRequest) (iosHelperResponse, error) {
	var response iosHelperResponse
	workspace := request.Options.Workspace
	if workspace == "" {
		var e error
		workspace, e = os.Getwd()
		if e != nil {
			return response, errors.New("ios_workspace_invalid")
		}
	}
	workspace, err := filepath.Abs(workspace)
	if err == nil {
		workspace, err = filepath.EvalSymlinks(workspace)
	}
	if err != nil {
		return response, errors.New("ios_workspace_invalid")
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return response, errors.New("ios_workspace_invalid")
	}
	request.Options.Workspace = workspace
	executable, err := os.Executable()
	if err != nil {
		return response, errors.New("ios_helper_unavailable")
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > 64*1024 {
		return response, errors.New("ios_helper_input_invalid")
	}
	output, err := toolOutput(ctx, toolCommand{Workspace: request.Options.Workspace, Executable: executable, Args: []string{"__ios-signing"}, Stdin: data})
	if err != nil {
		if errors.Is(err, errToolCleanup) {
			return response, ErrIOSCleanup
		}
		return response, errors.New("ios_helper_failed")
	}
	if len(output) > 16*1024 || json.Unmarshal([]byte(output), &response) != nil {
		return response, errors.New("ios_helper_response_invalid")
	}
	if response.Reason != "" {
		switch response.Reason {
		case "ios_cleanup_failed":
			return response, ErrIOSCleanup
		case "ios_signing_unsupported", "ios_material_invalid", "ios_profile_untrusted", "ios_profile_mismatch", "ios_p12_invalid", "ios_keychain_failed", "ios_partition_failed":
			return response, errors.New(response.Reason)
		default:
			return response, errors.New("ios_helper_failed")
		}
	}
	return response, nil
}

// PrepareIOSResources 保留本地便利入口，复用同一Plan/Prepare/Close真实生命周期。
func PrepareIOSResources(ctx context.Context, options IOSSigningOptions) (*IOSResources, error) {
	resources, err := PlanIOSResources(ctx, options)
	if err != nil {
		return nil, err
	}
	if err = resources.Prepare(ctx, options); err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), iosResourceBudget)
		defer cancel()
		if e := resources.Close(closeCtx); e != nil {
			err = errors.Join(err, ErrIOSCleanup)
		}
		return nil, err
	}
	return resources, nil
}

// Prepare 只有所有权已持久化后才可调用，不在恢复中重跑导入或构建。
func (resources *IOSResources) Prepare(ctx context.Context, options IOSSigningOptions) error {
	resources.mu.Lock()
	defer resources.mu.Unlock()
	if resources.closed || resources.preparing || resources.prepared || !iosOwnershipToken.MatchString(resources.token) {
		return errors.New("ios_resource_state_invalid")
	}
	if !iosNativeAvailable() {
		return errors.New("ios_signing_unsupported")
	}
	if e := validateIOSOptions(options); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("ios_prepare_cancelled")
	}
	workspace, e := filepath.Abs(options.Workspace)
	if e == nil {
		workspace, e = filepath.EvalSymlinks(workspace)
	}
	if e != nil || workspace != resources.workspace || filepath.Join(workspace, filepath.FromSlash(options.OutputDir)) != resources.output {
		return errors.New("ios_resource_state_invalid")
	}
	for _, item := range []struct {
		path string
		info os.FileInfo
	}{{resources.output, resources.outputInfo}, {resources.temporary, resources.temporaryInfo}} {
		actual, e := os.Lstat(item.path)
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(item.path))
		if parentErr != nil || parent != filepath.Dir(item.path) || e != nil || item.info == nil || !os.SameFile(actual, item.info) || !actual.IsDir() {
			return ErrIOSCleanup
		}
	}
	for _, filename := range []string{resources.keychain, resources.keychain + "-db", resources.profile} {
		if _, e := os.Lstat(filename); !os.IsNotExist(e) {
			return ErrIOSCleanup
		}
	}
	options.Workspace = workspace
	resources.preparing = true
	response, e := invokeIOSHelper(ctx, iosHelperRequest{Action: "prepare", Options: options, Keychain: resources.keychain})
	if !errors.Is(e, ErrIOSCleanup) {
		resources.preparing = false
	}
	resources.keychainInfo = map[string]os.FileInfo{}
	for _, filename := range []string{resources.keychain, resources.keychain + "-db"} {
		if info, err := os.Lstat(filename); err == nil && info.Mode().IsRegular() {
			resources.keychainInfo[filename] = info
		}
	}
	if e != nil {
		return e
	}
	if !iosProfileUUID.MatchString(response.UUID) || !iosTeamID.MatchString(response.Team) || len(response.Identity) != 40 {
		return errors.New("ios_helper_response_invalid")
	}
	if _, e = hex.DecodeString(response.Identity); e != nil {
		return errors.New("ios_helper_response_invalid")
	}
	resources.keychainInfo = map[string]os.FileInfo{}
	for _, filename := range []string{resources.keychain, resources.keychain + "-db"} {
		info, e := os.Lstat(filename)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !info.Mode().IsRegular() {
			return errors.New("ios_keychain_failed")
		}
		resources.keychainInfo[filename] = info
	}
	if len(resources.keychainInfo) == 0 {
		return errors.New("ios_keychain_failed")
	}

	// 材料文件在helper返回后重新读取只用于自有副本；摘要绑定由helper响应前完成。
	material, e := readIOSMaterial(options.ProfileFile, workspace, 2*1024*1024)
	if e != nil {
		return e
	}
	digest := sha256.Sum256(material)
	if hex.EncodeToString(digest[:]) != response.ProfileDigest {
		return errors.New("ios_material_changed")
	}
	profileDir := filepath.Dir(resources.profile)
	if e = os.MkdirAll(profileDir, 0700); e != nil {
		return errors.New("ios_profile_install_failed")
	}
	canonical, e := filepath.EvalSymlinks(profileDir)
	if e != nil || canonical != profileDir {
		return errors.New("ios_profile_install_failed")
	}
	file, e := os.OpenFile(resources.profile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("ios_profile_install_failed")
	}
	resources.profileInfo, e = file.Stat()
	written := 0
	if e == nil {
		written, e = file.Write(material)
	}
	resources.profileHash = sha256.Sum256(material[:written])
	closeErr := file.Close()
	if e != nil || closeErr != nil {
		return errors.New("ios_profile_install_failed")
	}
	resources.profileHash = sha256.Sum256(material)
	export := filepath.Join(resources.temporary, "ExportOptions.plist")
	if e = os.WriteFile(export, iosExportOptions(options, response), 0600); e != nil {
		return errors.New("ios_export_options_failed")
	}
	resources.env = map[string]string{"MYBUILDS_IOS_KEYCHAIN": resources.keychain, "MYBUILDS_IOS_SIGNING_IDENTITY": response.Identity, "MYBUILDS_IOS_PROFILE_UUID": response.UUID, "MYBUILDS_IOS_TEAM_ID": response.Team, "MYBUILDS_IOS_BUNDLE_ID": options.BundleID, "MYBUILDS_IOS_EXPORT_OPTIONS": export, "MYBUILDS_IOS_BUILD_DIR": resources.temporary, "MYBUILDS_IOS_OUTPUT_DIR": options.OutputDir}
	resources.team = response.Team
	resources.prepared = true
	return nil
}

func (resources *IOSResources) Environment() map[string]string {
	resources.mu.Lock()
	defer resources.mu.Unlock()
	env := map[string]string{}
	if !resources.closed {
		for key, value := range resources.env {
			env[key] = value
		}
	}
	return env
}

// Close 复核所有权后删除；被替换的profile或目录保留，不能恢复未知用户资源。
func (resources *IOSResources) Close(ctx context.Context) error {
	resources.mu.Lock()
	defer resources.mu.Unlock()
	if resources.closed {
		return nil
	}
	if resources.preparing {
		return ErrIOSCleanup
	}
	failed := false
	temporaryOwned := true
	if resources.temporary != "" {
		info, e := os.Lstat(resources.temporary)
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(resources.temporary))
		if !os.IsNotExist(e) && (e != nil || parentErr != nil || parent != filepath.Dir(resources.temporary) || resources.temporaryInfo == nil || !os.SameFile(info, resources.temporaryInfo)) {
			temporaryOwned = false
			failed = true
		}
	}
	keychainOwned := temporaryOwned
	keychainPresent := false
	if resources.keychain != "" {
		for _, filename := range []string{resources.keychain, resources.keychain + "-db"} {
			info, e := os.Lstat(filename)
			if os.IsNotExist(e) {
				continue
			}
			keychainPresent = true
			expected := resources.keychainInfo[filename]
			if e != nil || expected == nil || !os.SameFile(info, expected) || !info.Mode().IsRegular() {
				keychainOwned = false
				failed = true
			}
		}
	}
	if resources.keychain != "" && keychainOwned && keychainPresent {
		_, err := invokeIOSHelper(ctx, iosHelperRequest{Action: "close", Options: IOSSigningOptions{Workspace: resources.workspace}, Keychain: resources.keychain})
		if err != nil {
			failed = true
		}
	}
	if resources.profile != "" {
		info, err := os.Lstat(resources.profile)
		if !os.IsNotExist(err) {
			data, readErr := readIOSMaterial(resources.profile, resources.workspace, 2*1024*1024)
			if err == nil && info.Mode().IsRegular() && info.Size() == 0 {
				data = []byte{}
				readErr = nil
			}
			if err != nil || readErr != nil || resources.profileInfo == nil || !os.SameFile(info, resources.profileInfo) || sha256.Sum256(data) != resources.profileHash {
				failed = true
			} else if os.Remove(resources.profile) != nil {
				failed = true
			}
		}
	}
	for _, item := range []struct {
		path string
		info os.FileInfo
	}{{resources.temporary, resources.temporaryInfo}, {resources.output, resources.outputInfo}} {
		if item.path == "" {
			continue
		}
		info, err := os.Lstat(item.path)
		if os.IsNotExist(err) {
			continue
		}
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(item.path))
		if err != nil || parentErr != nil || parent != filepath.Dir(item.path) || item.info == nil || !os.SameFile(info, item.info) {
			failed = true
			continue
		}
		// keychain删除失败时保留证据；不能仅删除磁盘文件声称原生资源已清理。
		if item.path == resources.temporary && failed {
			continue
		}
		if os.RemoveAll(item.path) != nil {
			failed = true
		}
	}
	if failed {
		return ErrIOSCleanup
	}
	resources.closed = true
	resources.env = nil
	return nil
}

func iosRandomToken() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", errors.New("ios_random_failed")
	}
	return hex.EncodeToString(data), nil
}

// HandleIOSHelper 仅支持本功能的三个动作，任何失败均生成固定安全响应。
func HandleIOSHelper(input io.Reader, output io.Writer) error {
	var request iosHelperRequest
	var response iosHelperResponse
	data, err := io.ReadAll(io.LimitReader(input, 64*1024+1))
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	shapeErr := iosHelperJSON(tokens, 0, &count)
	if shapeErr != nil || err != nil || len(data) > 64*1024 || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		response.Reason = "ios_helper_input_invalid"
		_ = json.NewEncoder(output).Encode(response)
		return errors.New(response.Reason)
	}
	switch request.Action {
	case "inspect", "prepare":
		material, e := inspectIOSMaterial(request.Options)
		err = e
		if err == nil {
			response.Identity = material.identity
			response.UUID = material.uuid
			response.Team = material.team
			digest := sha256.Sum256(material.profile)
			response.ProfileDigest = hex.EncodeToString(digest[:])
		}
		if err == nil && request.Action == "prepare" {
			if !validIOSKeychainPath(request.Keychain) {
				err = errors.New("ios_keychain_failed")
			} else {
				p12 := material.p12
				keychainPassword, e := iosRandomToken()
				if err == nil {
					err = e
				}
				if err == nil {
					err = iosNativeImport(p12, request.Options.Password, request.Keychain, keychainPassword)
				}
				if err == nil {
					command := exec.Command("/usr/bin/security", "-i", "-q")
					command.Stdin = strings.NewReader("set-key-partition-list -S apple: -s -k " + keychainPassword + " \"" + request.Keychain + "\"\n")
					command.Stdout = io.Discard
					command.Stderr = io.Discard
					command.WaitDelay = time.Second
					if command.Run() != nil {
						err = errors.New("ios_partition_failed")
					}
				}
			}
		}
	case "close":
		if !validIOSKeychainPath(request.Keychain) {
			err = errors.New("ios_cleanup_failed")
		} else {
			err = iosNativeDelete(request.Keychain)
		}
	default:
		response.Reason = "ios_helper_input_invalid"
		_ = json.NewEncoder(output).Encode(response)
		return errors.New(response.Reason)
	}
	if err != nil {
		response = iosHelperResponse{Reason: err.Error()}
	}
	return json.NewEncoder(output).Encode(response)
}

func validIOSKeychainPath(keychain string) bool {
	return filepath.IsAbs(keychain) && filepath.Base(keychain) == "signing.keychain" && strings.HasPrefix(filepath.Base(filepath.Dir(keychain)), "mybuilds-ios-") && len(keychain) < 512 && !strings.ContainsAny(keychain, "\\\"\n\r\x00")
}

func readIOSMaterial(filename, workspace string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workspace, filename)
	}
	file, err := openIOSMaterial(filename)
	if err != nil {
		return nil, errors.New("ios_material_invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New("ios_material_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil, errors.New("ios_material_invalid")
	}
	return data, nil
}

func inspectIOSMaterial(options IOSSigningOptions) (iosMaterial, error) {
	var material iosMaterial
	if err := validateIOSOptions(options); err != nil {
		return material, errors.New("ios_material_invalid")
	}
	p12, err := readIOSMaterial(options.P12File, options.Workspace, 16*1024*1024)
	if err != nil {
		return material, err
	}
	profile, err := readIOSMaterial(options.ProfileFile, options.Workspace, 2*1024*1024)
	if err != nil {
		return material, err
	}
	certDER, plist, signerDER, err := iosNativeInspect(p12, profile, options.Password)
	if err != nil {
		return material, err
	}
	signer, err := x509.ParseCertificate(signerDER)
	if err != nil || signer.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return material, errors.New("ios_profile_untrusted")
	}
	marker := false
	for _, extension := range signer.Extensions {
		if extension.Id.String() == "1.2.840.113635.100.6.2.2.1" {
			marker = true
		}
	}
	if !marker {
		return material, errors.New("ios_profile_untrusted")
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil || time.Now().Before(cert.NotBefore) || !time.Now().Before(cert.NotAfter) {
		return material, errors.New("ios_p12_invalid")
	}
	decoder := xml.NewDecoder(bytes.NewReader(plist))
	var value any
	for {
		token, e := decoder.Token()
		if e != nil {
			return material, errors.New("ios_profile_mismatch")
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "dict" {
			value, err = decodeIOSPlist(decoder, start)
			break
		}
	}
	dictionary, ok := value.(map[string]any)
	if err != nil || !ok {
		return material, errors.New("ios_profile_mismatch")
	}
	uuid, _ := dictionary["UUID"].(string)
	teamArray, _ := dictionary["TeamIdentifier"].([]any)
	if len(teamArray) != 1 {
		return material, errors.New("ios_profile_mismatch")
	}
	team, _ := teamArray[0].(string)
	expiration, _ := dictionary["ExpirationDate"].(string)
	expires, e := time.Parse(time.RFC3339, expiration)
	entitlements, _ := dictionary["Entitlements"].(map[string]any)
	appID, _ := entitlements["application-identifier"].(string)
	entitlementTeam, _ := entitlements["com.apple.developer.team-identifier"].(string)
	if !iosProfileUUID.MatchString(uuid) || !iosTeamID.MatchString(team) || e != nil || !time.Now().Before(expires) || entitlementTeam != team || !(appID == team+"."+options.BundleID || appID == team+".*") {
		return material, errors.New("ios_profile_mismatch")
	}
	allowedCerts, _ := dictionary["DeveloperCertificates"].([]any)
	match := false
	for _, entry := range allowedCerts {
		encoded, _ := entry.(string)
		der, e := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(encoded), ""))
		if e == nil && bytes.Equal(der, certDER) {
			match = true
		}
	}
	getTaskAllow, _ := entitlements["get-task-allow"].(bool)
	allDevices, _ := dictionary["ProvisionsAllDevices"].(bool)
	devices, _ := dictionary["ProvisionedDevices"].([]any)
	methodMatch := false
	switch options.ExportMethod {
	case "debugging":
		methodMatch = getTaskAllow && len(devices) > 0
	case "release-testing":
		methodMatch = !getTaskAllow && len(devices) > 0 && !allDevices
	case "enterprise":
		methodMatch = !getTaskAllow && allDevices
	case "app-store-connect":
		methodMatch = !getTaskAllow && !allDevices && len(devices) == 0
	}
	if !match || !methodMatch {
		return material, errors.New("ios_profile_mismatch")
	}
	fingerprint := sha1.Sum(certDER)
	material = iosMaterial{identity: strings.ToUpper(hex.EncodeToString(fingerprint[:])), uuid: uuid, team: team, profile: profile, p12: p12}
	return material, nil
}

// decodeIOSPlist 仅为Security已验证内容读取plist值，不运行外部解析命令。
func decodeIOSPlist(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		values := map[string]any{}
		var key string
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch current := token.(type) {
			case xml.EndElement:
				return values, nil
			case xml.StartElement:
				if current.Name.Local == "key" {
					if key != "" {
						return nil, errors.New("plist_invalid")
					}
					if err = decoder.DecodeElement(&key, &current); err != nil {
						return nil, err
					}
				} else {
					if key == "" {
						return nil, errors.New("plist_invalid")
					}
					value, e := decodeIOSPlist(decoder, current)
					if e != nil {
						return nil, e
					}
					if _, exists := values[key]; exists {
						return nil, errors.New("plist_invalid")
					}
					values[key] = value
					key = ""
				}
			}
		}
	case "array":
		values := []any{}
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch current := token.(type) {
			case xml.EndElement:
				return values, nil
			case xml.StartElement:
				value, e := decodeIOSPlist(decoder, current)
				if e != nil {
					return nil, e
				}
				values = append(values, value)
			}
		}
	case "true", "false":
		if err := decoder.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	default:
		var value string
		err := decoder.DecodeElement(&value, &start)
		return value, err
	}
}

func iosExportOptions(options IOSSigningOptions, response iosHelperResponse) []byte {
	var method, bundle, team, uuid, identity bytes.Buffer
	for _, item := range []struct {
		buffer *bytes.Buffer
		value  string
	}{{&method, options.ExportMethod}, {&bundle, options.BundleID}, {&team, response.Team}, {&uuid, response.UUID}, {&identity, response.Identity}} {
		_ = xml.EscapeText(item.buffer, []byte(item.value))
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>method</key><string>` + method.String() + `</string><key>destination</key><string>export</string><key>signingStyle</key><string>manual</string><key>teamID</key><string>` + team.String() + `</string><key>signingCertificate</key><string>` + identity.String() + `</string><key>provisioningProfiles</key><dict><key>` + bundle.String() + `</key><string>` + uuid.String() + `</string></dict><key>manageAppVersionAndBuildNumber</key><false/></dict></plist>`)
}

// helper只接收本功能的有限JSON；重复键、null及深层输入不采用后值解释。
func iosHelperJSON(dec *json.Decoder, depth int, count *int) error {
	*count++
	if depth > 32 || *count > 10000 {
		return errors.New("ios_helper_input_invalid")
	}
	value, err := dec.Token()
	if err != nil || value == nil {
		return errors.New("ios_helper_input_invalid")
	}
	delim, ok := value.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			value, e := dec.Token()
			key, ok := value.(string)
			if e != nil || !ok || seen[strings.ToLower(key)] {
				return errors.New("ios_helper_input_invalid")
			}
			seen[strings.ToLower(key)] = true
			if e = iosHelperJSON(dec, depth+1, count); e != nil {
				return e
			}
		}
		value, err = dec.Token()
		if err != nil || value != json.Delim('}') {
			return errors.New("ios_helper_input_invalid")
		}
	case '[':
		for dec.More() {
			if err = iosHelperJSON(dec, depth+1, count); err != nil {
				return err
			}
		}
		value, err = dec.Token()
		if err != nil || value != json.Delim(']') {
			return errors.New("ios_helper_input_invalid")
		}
	default:
		return errors.New("ios_helper_input_invalid")
	}
	return nil
}
