package distribute

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func TestActualPrivateKeyCredentialValidation(t *testing.T) {
	rsaKey, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(rsaKey)
	g := googleKey{Type: "service_account", TokenURI: "https://oauth2.googleapis.com/token", ClientEmail: "own@own.iam.gserviceaccount.com", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))}
	b, _ := json.Marshal(g)
	if _, e = googleCredential(b); e != nil {
		t.Fatal("真实RSA拒绝")
	}
	g.TokenURI = "http://127.0.0.1/token"
	b, _ = json.Marshal(g)
	if _, e = googleCredential(b); e == nil {
		t.Fatal("外部token origin")
	}
	ec, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	der, _ = x509.MarshalPKCS8PrivateKey(ec)
	a := appleKey{KeyID: "OWNKEY123", IssuerID: "aabbccdd-1122-3344-5566-778899aabbcc", Key: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), Duration: 200}
	b, _ = json.Marshal(a)
	if _, e = appleCredential(b); e != nil {
		t.Fatal("真实P256拒绝")
	}
	for _, suffix := range []string{`,"duration":0`, `,"duration":1201`, `,"duration":0.1`, `,"duration":null`, `,"in_house":true`, `,"key_filepath":"unknown"`, `,"Key":"unknown"`} {
		var raw map[string]any
		json.Unmarshal(b, &raw)
		delete(raw, "duration")
		delete(raw, "in_house")
		base, _ := json.Marshal(raw)
		changed := strings.TrimSuffix(string(base), "}") + suffix + "}"
		if _, e = appleCredential([]byte(changed)); e == nil {
			t.Fatal("非法Apple材料")
		}
	}
}
