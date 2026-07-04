package middleware

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

type recordingLogger struct {
	mu      *sync.Mutex
	records *[]logRecord
	attrs   []any
}

type logRecord struct {
	level string
	msg   string
	attrs []any
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{
		mu:      &sync.Mutex{},
		records: &[]logRecord{},
	}
}

func (l *recordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &recordingLogger{mu: l.mu, records: l.records, attrs: attrs}
}

func (l *recordingLogger) Debug(msg string, args ...any) { l.append("debug", msg, args...) }
func (l *recordingLogger) Info(msg string, args ...any)  { l.append("info", msg, args...) }
func (l *recordingLogger) Warn(msg string, args ...any)  { l.append("warn", msg, args...) }
func (l *recordingLogger) Error(msg string, args ...any) { l.append("error", msg, args...) }

func (l *recordingLogger) append(level, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attrs := append(append([]any{}, l.attrs...), args...)
	*l.records = append(*l.records, logRecord{level: level, msg: msg, attrs: attrs})
}

func (l *recordingLogger) snapshot() []logRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]logRecord, len(*l.records))
	copy(out, *l.records)
	return out
}

func attrString(t *testing.T, attrs []any, key string) string {
	t.Helper()
	value, ok := attrValue(attrs, key)
	require.Truef(t, ok, "missing attr %s", key)
	text, ok := value.(string)
	require.Truef(t, ok, "attr %s is %T", key, value)
	return text
}

func attrBool(t *testing.T, attrs []any, key string) bool {
	t.Helper()
	value, ok := attrValue(attrs, key)
	require.Truef(t, ok, "missing attr %s", key)
	flag, ok := value.(bool)
	require.Truef(t, ok, "attr %s is %T", key, value)
	return flag
}

func attrMap(t *testing.T, attrs []any, key string) map[string]any {
	t.Helper()
	value, ok := attrValue(attrs, key)
	require.Truef(t, ok, "missing attr %s", key)
	asMap, ok := value.(map[string]any)
	require.Truef(t, ok, "attr %s is %T", key, value)
	return asMap
}

func headerValue(t *testing.T, headers map[string]any, name string) any {
	t.Helper()
	if value, ok := headers[name]; ok {
		return value
	}

	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}

	t.Fatalf("missing header %s in %#v", name, headers)
	return nil
}

func attrValue(attrs []any, key string) (any, bool) {
	for i := 0; i+1 < len(attrs); i += 2 {
		name, ok := attrs[i].(string)
		if !ok || name != key {
			continue
		}

		return attrs[i+1], true
	}

	return nil, false
}

func attrsContainString(attrs []any, needle string) bool {
	for _, attr := range attrs {
		if text, ok := attr.(string); ok && strings.Contains(text, needle) {
			return true
		}

		if nested, ok := attr.(map[string]any); ok && mapContainsString(nested, needle) {
			return true
		}
	}

	return false
}

func mapContainsString(value map[string]any, needle string) bool {
	for _, child := range value {
		switch typed := child.(type) {
		case string:
			if strings.Contains(typed, needle) {
				return true
			}

		case map[string]any:
			if mapContainsString(typed, needle) {
				return true
			}

		case []string:
			for _, item := range typed {
				if strings.Contains(item, needle) {
					return true
				}
			}

		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.Contains(text, needle) {
					return true
				}
			}
		}
	}

	return false
}
