package distribute

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var errMaterial = errors.New("publish_material_invalid")

func readMaterial(ctx context.Context, path string, limit int64, secret bool) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errMaterial
	}
	f, e := openMaterial(path, secret)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil || before.Size() > limit {
		return nil, errMaterial
	}
	data, e := io.ReadAll(io.LimitReader(f, limit+1))
	after, ae := f.Stat()
	leaf, le := os.Lstat(path)
	if e != nil || ae != nil || le != nil || !os.SameFile(before, after) || !os.SameFile(after, leaf) || leaf.Mode()&os.ModeSymlink != 0 || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != after.Size() || ctx.Err() != nil {
		return nil, errMaterial
	}
	return data, nil
}

// 所有三个具体 JSON 消费者均拒绝重复键，不依赖encoding/json的后值覆盖。
func strictJSON(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var visit func() error
	visit = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		if t == nil {
			return errMaterial
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, e := d.Token()
					if e != nil {
						return e
					}
					s, ok := key.(string)
					if !ok || seen[s] {
						return errMaterial
					}
					seen[s] = true
					if e = visit(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			case '[':
				for d.More() {
					if e = visit(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			default:
				return errMaterial
			}
		}
		return nil
	}
	if visit() != nil {
		return errMaterial
	}
	if _, e := d.Token(); e != io.EOF {
		return errMaterial
	}
	if !exactFields(data, reflect.TypeOf(out).Elem()) {
		return errMaterial
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return errMaterial
	}
	return nil
}

type googleKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
	AuthProvider string `json:"auth_provider_x509_cert_url"`
	ClientCert   string `json:"client_x509_cert_url"`
	Universe     string `json:"universe_domain"`
}

func googleCredential(data []byte) (googleKey, error) {
	var k googleKey
	if strictJSON(data, &k) != nil || k.Type != "service_account" || k.TokenURI != "https://oauth2.googleapis.com/token" || !strings.HasSuffix(k.ClientEmail, ".iam.gserviceaccount.com") || strings.ContainsAny(k.ClientEmail, "\r\n\x00") {
		return k, errMaterial
	}
	p, rest := pem.Decode([]byte(k.PrivateKey))
	if p == nil || len(bytes.TrimSpace(rest)) != 0 || p.Type != "PRIVATE KEY" {
		return k, errMaterial
	}
	key, e := x509.ParsePKCS8PrivateKey(p.Bytes)
	r, ok := key.(*rsa.PrivateKey)
	if e != nil || !ok || r.N.BitLen() < 2048 {
		return k, errMaterial
	}
	return k, nil
}

type appleKey struct {
	KeyID    string `json:"key_id"`
	IssuerID string `json:"issuer_id"`
	Key      string `json:"key"`
	Duration int64  `json:"duration,omitempty"`
	InHouse  bool   `json:"in_house,omitempty"`
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var appPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func appleCredential(data []byte) (appleKey, error) {
	var k appleKey
	if strictJSON(data, &k) != nil || !idPattern.MatchString(k.KeyID) || !uuidPattern.MatchString(k.IssuerID) || k.InHouse {
		return k, errMaterial
	}
	var declared map[string]json.RawMessage
	_ = json.Unmarshal(data, &declared)
	if raw, ok := declared["duration"]; ok && string(raw) == "0" {
		return k, errMaterial
	}
	if k.Duration == 0 {
		k.Duration = 200
	}
	if k.Duration < 1 || k.Duration > 1200 {
		return k, errMaterial
	}
	p, rest := pem.Decode([]byte(k.Key))
	if p == nil || p.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return k, errMaterial
	}
	key, e := x509.ParsePKCS8PrivateKey(p.Bytes)
	ec, ok := key.(*ecdsa.PrivateKey)
	if e != nil || !ok || ec.Curve != elliptic.P256() {
		return k, errMaterial
	}
	return k, nil
}

func exactFields(data []byte, t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return true
	}
	switch t.Kind() {
	case reflect.Struct:
		var m map[string]json.RawMessage
		if json.Unmarshal(data, &m) != nil {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		for name, v := range m {
			typ, ok := fields[name]
			if !ok || !exactFields(v, typ) {
				return false
			}
		}
	case reflect.Slice:
		var rows []json.RawMessage
		if json.Unmarshal(data, &rows) != nil {
			return false
		}
		for _, v := range rows {
			if !exactFields(v, t.Elem()) {
				return false
			}
		}
	}
	return true
}
