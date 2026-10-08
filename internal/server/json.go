package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"mybuilds/internal/scm"
	"mybuilds/internal/store"
)

var errTooLarge = errors.New("request_too_large")
var errPipeline = errors.New("invalid_pipeline")
var errUnsupported = errors.New("unsupported_setting")

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func writeError(w http.ResponseWriter, err error) {
	status, code, message := 500, "internal_error", "服务处理失败"
	switch {
	case errors.Is(err, store.ErrRetentionRetired):
		status, code, message = 410, "retention_retired", "构建证据已退役"
	case errors.Is(err, store.ErrRetentionProtected), errors.Is(err, store.ErrRetentionReadersActive), errors.Is(err, store.ErrRetentionOwnershipUnknown), errors.Is(err, store.ErrRetentionReceiptConflict):
		status, message = 409, "清理状态或归属无法确认"
		for _, known := range []error{store.ErrRetentionProtected, store.ErrRetentionReadersActive, store.ErrRetentionOwnershipUnknown, store.ErrRetentionReceiptConflict} {
			if errors.Is(err, known) {
				code = known.Error()
				break
			}
		}
	case errors.Is(err, store.ErrRetentionObjectInvalid), errors.Is(err, store.ErrRetentionLimit):
		status, message = 400, "清理请求无效"
		if errors.Is(err, store.ErrRetentionObjectInvalid) {
			code = "retention_object_invalid"
		} else {
			code = "retention_limit"
		}
	case errors.Is(err, store.ErrRetentionIO):
		status, code, message = 409, "retention_io_error", "清理文件操作未确认"
	case errors.Is(err, store.ErrRetentionInvalid):
		status, code, message = 400, "retention_invalid", "保留策略无效"
	case errors.Is(err, store.ErrRetentionTimeout):
		status, code, message = 504, "retention_timeout", "保留策略操作超时"
	case errors.Is(err, store.ErrRetentionCancelled):
		status, code, message = 409, "retention_cancelled", "保留策略操作已取消"
	case errors.Is(err, store.ErrInvalid):
		status, code, message = 400, "invalid_request", "请求无效"
	case errors.Is(err, errPipeline):
		status, code, message = 400, "invalid_pipeline", "流水线无效"
	case errors.Is(err, errUnsupported):
		status, code, message = 400, "unsupported_setting", "当前设置尚未支持"
	case errors.Is(err, store.ErrNodeUnauthorized):
		status, code, message = 401, "node_unauthorized", "节点身份无效"
	case errors.Is(err, store.ErrSessionConflict), errors.Is(err, store.ErrSessionExpired), errors.Is(err, store.ErrLeaseInvalid), errors.Is(err, store.ErrLeaseExpired), errors.Is(err, store.ErrEventConflict), errors.Is(err, store.ErrSequenceInvalid), errors.Is(err, store.ErrStopUnconfirmed), errors.Is(err, store.ErrArtifactConflict), errors.Is(err, store.ErrLogConflict):
		status, code, message = 409, "conflict", "请求与执行权或证据冲突"
		for _, known := range []error{store.ErrSessionConflict, store.ErrSessionExpired, store.ErrLeaseInvalid, store.ErrLeaseExpired, store.ErrEventConflict, store.ErrSequenceInvalid, store.ErrStopUnconfirmed, store.ErrArtifactConflict, store.ErrLogConflict} {
			if errors.Is(err, known) {
				code = known.Error()
				break
			}
		}
	case errors.Is(err, store.ErrBudgetInvalid):
		status, code, message = 400, "budget_invalid", "预算不合法"
	case errors.Is(err, store.ErrUnauthorized):
		status, code, message = 401, "unauthorized", "身份无效"
	case errors.Is(err, store.ErrForbidden):
		status, code, message = 403, "forbidden", "无权执行此操作"
	case errors.Is(err, store.ErrNotFound):
		status, code, message = 404, "not_found", "对象或入口不存在"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "conflict", "请求与当前状态冲突"
	case errors.Is(err, store.ErrLockLost):
		status, code, message = 503, "control_lock_lost", "控制端运行权已丢失"
	case errors.Is(err, errTooLarge):
		status, code, message = 413, "request_too_large", "请求超过大小上限"
	default:
		var git *scm.Error
		if errors.As(err, &git) {
			status, code, message = 400, "invalid_pipeline", "无法读取有效流水线"
		}
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// 先遍历原始Token查重复与null，再按声明的精确JSON字段检查；Decoder不承担弱结构校验。
func readJSON(r *http.Request, target any) error {
	return readJSONLimit(r, target, 1<<20)
}
func readJSONLimit(r *http.Request, target any, limit int64) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return store.ErrInvalid
	}
	if int64(len(data)) > limit {
		return errTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	nodes := 0
	maxNodes := 10000
	if limit > 1<<20 {
		maxNodes = 65536
	}
	value, err := jsonValue(dec, 0, &nodes, maxNodes)
	if err != nil {
		return store.ErrInvalid
	}
	if _, err = dec.Token(); err != io.EOF {
		return store.ErrInvalid
	}
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Pointer || checkJSONFields(value, typ.Elem()) != nil {
		return store.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return store.ErrInvalid
	}
	return nil
}
func jsonValue(dec *json.Decoder, depth int, nodes *int, maxNodes int) (any, error) {
	*nodes++
	if depth > 64 || *nodes > maxNodes {
		return nil, store.ErrInvalid
	}
	token, err := dec.Token()
	if err != nil || token == nil {
		return nil, store.ErrInvalid
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		result := map[string]any{}
		for dec.More() {
			key, err := dec.Token()
			name, ok := key.(string)
			if err != nil || !ok {
				return nil, store.ErrInvalid
			}
			if _, exists := result[name]; exists {
				return nil, store.ErrInvalid
			}
			value, err := jsonValue(dec, depth+1, nodes, maxNodes)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim('}') {
			return nil, store.ErrInvalid
		}
		return result, nil
	case '[':
		result := []any{}
		for dec.More() {
			value, err := jsonValue(dec, depth+1, nodes, maxNodes)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim(']') {
			return nil, store.ErrInvalid
		}
		return result, nil
	default:
		return nil, store.ErrInvalid
	}
}
func checkJSONFields(value any, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeOf(time.Time{}) {
		value, ok := value.(string)
		if !ok {
			return store.ErrInvalid
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return store.ErrInvalid
		}
		_, offset := parsed.Zone()
		if offset != 0 {
			return store.ErrInvalid
		}
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		mapping, ok := value.(map[string]any)
		if !ok {
			return store.ErrInvalid
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			if name != "-" {
				fields[name] = field.Type
			}
		}
		for name, item := range mapping {
			field, exists := fields[name]
			if !exists || checkJSONFields(item, field) != nil {
				return store.ErrInvalid
			}
		}
	case reflect.Map:
		mapping, ok := value.(map[string]any)
		if !ok {
			return store.ErrInvalid
		}
		for _, item := range mapping {
			if checkJSONFields(item, typ.Elem()) != nil {
				return store.ErrInvalid
			}
		}
	case reflect.Slice:
		items, ok := value.([]any)
		if !ok {
			return store.ErrInvalid
		}
		for _, item := range items {
			if checkJSONFields(item, typ.Elem()) != nil {
				return store.ErrInvalid
			}
		}
	}
	return nil
}
func readQuery(r *http.Request, allowed ...string) (map[string]string, error) {
	// ParseQuery拒绝非法编码与分号。
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, store.ErrInvalid
	}
	result := map[string]string{}
	for name, items := range values {
		known := false
		for _, key := range allowed {
			known = known || name == key
		}
		if !known || len(items) != 1 {
			return nil, store.ErrInvalid
		}
		result[name] = items[0]
	}
	return result, nil
}
func readPage(values map[string]string) (store.Page, error) {
	p := store.Page{Limit: 20}
	var err error
	if v, ok := values["limit"]; ok {
		p.Limit, err = strconv.Atoi(v)
		if err != nil || p.Limit < 1 || p.Limit > 200 {
			return p, store.ErrInvalid
		}
	}
	if v, ok := values["offset"]; ok {
		p.Offset, err = strconv.Atoi(v)
		if err != nil || p.Offset < 0 || p.Offset > 1000000 {
			return p, store.ErrInvalid
		}
	}
	return p, nil
}
