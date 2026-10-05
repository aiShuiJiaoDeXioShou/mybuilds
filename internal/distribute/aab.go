package distribute

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mybuilds/internal/process"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errArtifact = errors.New("publish_artifact_invalid")

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	return r.r.Read(b)
}
func (p *prepared) copyArtifact(ctx context.Context, path, digest string, size int64, ext string) error {
	if !digestPattern.MatchString(digest) || size <= 0 || size > 1<<30 || strings.ToLower(filepath.Ext(path)) != ext {
		return errArtifact
	}
	f, e := openMaterial(path, false)
	if e != nil {
		return errArtifact
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil || before.Size() != size {
		return errArtifact
	}
	target := filepath.Join(p.workDir, "artifact"+ext)
	to, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return errArtifact
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(to, h), io.LimitReader(contextReader{ctx, f}, size+1))
	if e == nil {
		e = to.Sync()
	}
	ce := to.Close()
	after, ae := f.Stat()
	leaf, le := os.Lstat(path)
	if e != nil || ce != nil || ae != nil || le != nil || !os.SameFile(before, after) || !os.SameFile(after, leaf) || !before.ModTime().Equal(after.ModTime()) || n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		return errArtifact
	}
	p.artifactCopy = target
	return nil
}
func (p *prepared) tool(ctx context.Context, path string, args ...string) ([]byte, error) {
	resolved, e := exec.LookPath(path)
	if e != nil {
		return nil, errTools
	}
	var out, stderr limitedOutput
	r := process.Run(ctx, process.Command{Path: resolved, Args: args, Dir: p.workDir, Env: p.environment("")}, &out, &stderr)
	if r.CleanupFailed {
		p.mu.Lock()
		p.unsafe = true
		p.mu.Unlock()
		return nil, ErrCleanup
	}
	if r.ExitCode != 0 || r.Reason != "" {
		return nil, errArtifact
	}
	return append(out.data.Bytes(), stderr.data.Bytes()...), nil
}
func zipStructure(file string) error {
	z, e := zip.OpenReader(file)
	if e != nil {
		return errArtifact
	}
	defer z.Close()
	if len(z.File) > 50000 {
		return errArtifact
	}
	var total uint64
	names := map[string]bool{}
	for _, f := range z.File {
		n := f.Name
		if !utf8.ValidString(n) || strings.IndexFunc(n, unicode.IsControl) >= 0 || n == "" || strings.Contains(n, "\\") || strings.HasPrefix(n, "/") || strings.ContainsAny(n, "\x00\r\n") || names[n] {
			return errArtifact
		}
		names[n] = true
		for _, part := range strings.Split(n, "/") {
			if part == ".." {
				return errArtifact
			}
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return errArtifact
		}
		if f.UncompressedSize64 > 4<<30 {
			return errArtifact
		}
		total += f.UncompressedSize64
		if total > 4<<30 {
			return errArtifact
		}
	}
	return nil
}
func (p *prepared) validateAAB(ctx context.Context, in GooglePlayOptions) error {
	if zipStructure(p.artifactCopy) != nil {
		return errArtifact
	}
	jar, e := readMaterial(ctx, in.Bundletool, 64<<20, false)
	if e != nil {
		return errTools
	}
	jarHash := sha256.Sum256(jar)
	if hex.EncodeToString(jarHash[:]) != "a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29" {
		return errTools
	}
	version, e := p.tool(ctx, "java", "-jar", in.Bundletool, "version")
	if e != nil || strings.TrimSpace(string(version)) != "1.18.3" {
		return errTools
	}
	if _, e = p.tool(ctx, "java", "-jar", in.Bundletool, "validate", "--bundle="+p.artifactCopy); e != nil {
		return errArtifact
	}
	manifest, e := p.tool(ctx, "java", "-jar", in.Bundletool, "dump", "manifest", "--bundle="+p.artifactCopy, "--module=base")
	if e != nil {
		return errArtifact
	}
	d := xml.NewDecoder(strings.NewReader(string(manifest)))
	found := false
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return errArtifact
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "manifest" {
			continue
		}
		vals := map[string]string{}
		for _, a := range start.Attr {
			vals[a.Name.Local] = a.Value
		}
		if vals["package"] != in.AppIdentifier || vals["versionName"] != in.VersionName || vals["versionCode"] != strconv.FormatInt(in.Number, 10) {
			return errArtifact
		}
		found = true
		break
	}
	if !found {
		return errArtifact
	}
	signed, e := p.tool(ctx, "jarsigner", "-verify", p.artifactCopy)
	if e != nil || !strings.Contains(string(signed), "jar verified.") || strings.Contains(string(signed), "unsigned entries") {
		return errArtifact
	}
	cert, e := p.tool(ctx, "keytool", "-printcert", "-jarfile", p.artifactCopy)
	if e != nil {
		return errArtifact
	}
	want := strings.ToLower(strings.ReplaceAll(in.UploadCertificateSHA256, ":", ""))
	if !digestPattern.MatchString(want) {
		return errArtifact
	}
	match := false
	for _, line := range strings.Split(string(cert), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "SHA256:") {
			match = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(s, "SHA256:")), ":", "")) == want
			break
		}
	}
	if !match {
		return fmt.Errorf("%w", errArtifact)
	}
	return nil
}

// verifyArtifact在授权后的实际启动前再次核对本次已准备私有包，拒绝被替换的叶子。
func (p *prepared) verifyArtifact(ctx context.Context, digest string, size int64) error {
	f, e := openMaterial(p.artifactCopy, true)
	if e != nil {
		return errArtifact
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil || before.Size() != size {
		return errArtifact
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(contextReader{ctx, f}, size+1))
	after, ae := f.Stat()
	leaf, le := os.Lstat(p.artifactCopy)
	if e != nil || ae != nil || le != nil || n != size || !os.SameFile(before, after) || !os.SameFile(after, leaf) || !before.ModTime().Equal(after.ModTime()) || hex.EncodeToString(h.Sum(nil)) != digest {
		return errArtifact
	}
	return nil
}
