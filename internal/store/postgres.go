package store

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// 只接受当前控制端实际消费的参数，文件和宿主环境不交给 pgx 隐式读取。
var postgresKeys = []string{
	"host", "port", "user", "database", "password", "connect_timeout", "sslmode", "sslrootcert", "sslcert", "sslkey", "sslpassword", "sslsni", "sslnegotiation", "application_name", "search_path", "timezone", "options", "target_session_attrs", "channel_binding", "require_auth", "min_protocol_version", "max_protocol_version", "statement_cache_capacity", "description_cache_capacity", "default_query_exec_mode",
}

func postgresConfig(ctx context.Context, dsn string) (*pgx.ConnConfig, error) {
	if len(dsn) == 0 || len(dsn) > 1<<20 {
		return nil, ErrInvalid
	}
	// pgx 的白名单只覆盖 DSN；不能通过修改进程全局环境影响并发消费者。
	for _, value := range os.Environ() {
		name, content, _ := strings.Cut(value, "=")
		if content != "" && (strings.HasPrefix(name, "PG") || name == "SSL_CERT_FILE" || name == "SSL_CERT_DIR") {
			return nil, ErrInvalid
		}
	}
	settings, err := postgresSettings(dsn)
	if err != nil {
		return nil, ErrInvalid
	}
	allowed := make(map[string]bool, len(postgresKeys))
	for _, key := range postgresKeys {
		allowed[key] = true
	}
	for key := range settings {
		if !allowed[key] {
			return nil, ErrInvalid
		}
	}
	if ctx.Err() != nil {
		return nil, errDatabase
	}
	caPath, certPath, keyPath, password := settings["sslrootcert"], settings["sslcert"], settings["sslkey"], settings["sslpassword"]
	if (certPath == "") != (keyPath == "") {
		return nil, ErrInvalid
	}
	var roots *x509.CertPool
	if caPath == "system" {
		roots, err = x509.SystemCertPool()
		if err != nil {
			return nil, ErrInvalid
		}
		settings["sslmode"] = "verify-full"
	} else if caPath != "" {
		data, err := readPostgresFile(ctx, caPath, false)
		if err != nil {
			return nil, err
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, ErrInvalid
		}
		if settings["sslmode"] == "require" {
			settings["sslmode"] = "verify-ca"
		}
	}
	var certificate *tls.Certificate
	if certPath != "" {
		cert, err := readPostgresFile(ctx, certPath, false)
		if err != nil {
			return nil, err
		}
		key, err := readPostgresFile(ctx, keyPath, true)
		if err != nil {
			return nil, err
		}
		if block, _ := pem.Decode(key); block != nil && x509.IsEncryptedPEMBlock(block) {
			if password == "" {
				return nil, ErrInvalid
			}
			decoded, err := x509.DecryptPEMBlock(block, []byte(password))
			if err != nil {
				return nil, ErrInvalid
			}
			key = pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: decoded})
		}
		pair, err := tls.X509KeyPair(cert, key)
		if err != nil {
			return nil, ErrInvalid
		}
		certificate = &pair
	}
	// 显式空值覆盖 ~/.pgpass 和 ~/.postgresql 默认路径；禁用 service 先由白名单完成。
	for _, key := range []string{"passfile", "sslrootcert", "sslcert", "sslkey", "sslpassword"} {
		settings[key] = ""
	}
	if _, exists := settings["password"]; !exists {
		settings["password"] = ""
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var normalized strings.Builder
	for _, key := range keys {
		normalized.WriteString(key)
		normalized.WriteString("='")
		value := strings.ReplaceAll(strings.ReplaceAll(settings[key], "\\", "\\\\"), "'", "\\'")
		normalized.WriteString(value)
		normalized.WriteString("' ")
	}
	opts := pgx.ParseConfigOptions{ParseConfigOptions: pgconn.ParseConfigOptions{ConnStringAllowedKeys: append(append([]string{}, postgresKeys...), "passfile")}}
	config, err := pgx.ParseConfigWithOptions(normalized.String(), opts)
	if err != nil {
		return nil, ErrInvalid
	}
	configureTLS := func(t *tls.Config) {
		if t == nil {
			return
		}
		t.RootCAs = roots
		if certificate != nil {
			t.Certificates = []tls.Certificate{*certificate}
		}
		if settings["sslmode"] == "verify-ca" {
			t.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error { return verifyPostgresChain(raw, roots) }
		}
	}
	configureTLS(config.TLSConfig)
	for _, fallback := range config.Fallbacks {
		configureTLS(fallback.TLSConfig)
	}
	if ctx.Err() != nil {
		return nil, errDatabase
	}
	return config, nil
}

func verifyPostgresChain(raw [][]byte, roots *x509.CertPool) error {
	if len(raw) == 0 {
		return ErrInvalid
	}
	certs := make([]*x509.Certificate, 0, len(raw))
	for _, data := range raw {
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			return ErrInvalid
		}
		certs = append(certs, cert)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	if _, err := certs[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates}); err != nil {
		return ErrInvalid
	}
	return nil
}

// 仅提取标准 DSN 字段；值验证和连接配置仍使用实际 pgx。
func postgresSettings(dsn string) (map[string]string, error) {
	settings := map[string]string{}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil || u.Fragment != "" {
			return nil, ErrInvalid
		}
		if u.User != nil {
			settings["user"] = u.User.Username()
			if value, present := u.User.Password(); present {
				settings["password"] = value
			}
		}
		if u.Host != "" {
			settings["host"] = u.Host
		}
		if u.Path != "" {
			settings["database"] = strings.TrimLeft(u.Path, "/")
		}
		if u.Host != "" {
			var hosts, ports []string
			for _, host := range strings.Split(u.Host, ",") {
				part, err := url.Parse("postgres://" + host)
				if err != nil {
					return nil, ErrInvalid
				}
				hosts = append(hosts, part.Hostname())
				port := part.Port()
				if port == "" {
					port = "5432"
				}
				ports = append(ports, port)
			}
			settings["host"] = strings.Join(hosts, ",")
			settings["port"] = strings.Join(ports, ",")
		}
		// 与 pgx 一致，显式 URL query 覆盖 authority/path。
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return nil, ErrInvalid
		}
		for key, values := range query {
			if len(values) != 1 {
				return nil, ErrInvalid
			}
			settings[key] = values[0]
		}
	} else {
		input := strings.TrimSpace(dsn)
		for input != "" {
			at := strings.IndexByte(input, '=')
			if at < 1 {
				return nil, ErrInvalid
			}
			key := strings.TrimSpace(input[:at])
			if strings.ContainsAny(key, " \t\r\n\v\f") {
				return nil, ErrInvalid
			}
			input = strings.TrimLeft(input[at+1:], " \t\r\n\v\f")
			quoted := strings.HasPrefix(input, "'")
			if quoted {
				input = input[1:]
			}
			var value strings.Builder
			closed := !quoted
			for len(input) > 0 {
				c := input[0]
				input = input[1:]
				if c == '\\' {
					if len(input) == 0 {
						return nil, ErrInvalid
					}
					if input[0] != '\\' && input[0] != '\'' {
						value.WriteByte('\\')
					}
					value.WriteByte(input[0])
					input = input[1:]
					continue
				}
				if quoted && c == '\'' {
					closed = true
					break
				}
				if !quoted && strings.ContainsRune(" \t\r\n\v\f", rune(c)) {
					break
				}
				value.WriteByte(c)
			}
			if !closed {
				return nil, ErrInvalid
			}
			settings[key] = value.String()
			input = strings.TrimLeft(input, " \t\r\n\v\f")
		}
	}
	if value, exists := settings["dbname"]; exists {
		settings["database"] = value
		delete(settings, "dbname")
	}
	return settings, nil
}
