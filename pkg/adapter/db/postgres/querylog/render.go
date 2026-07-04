package querylog

import (
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Render returns a logging-only SQL representation with positional pgx
// placeholders ($1, $2, ...) replaced by SQL literals derived from args.
//
// The returned string must never be executed. Actual database calls remain
// parameterized through pgx; Render exists only so development logs show the
// effective values next to the statement text.
//
// Placeholder-like text inside single-quoted strings, double-quoted
// identifiers, -- / /* */ comments, and dollar-quoted bodies is left untouched
// so rendering cannot corrupt those regions or invent false substitutions.
//
// Capacity: the builder is pre-sized to len(sql) + len(args)*16. The *16 term is
// a rough average for formatted literals (quotes, casts such as ::timestamptz,
// UUID text, short numbers). It reduces reallocations while building the log
// string; it is not a correctness constraint and is unrelated to any wire
// protocol buffer size.
func Render(sql string, args []any) (string, error) {
	var out strings.Builder
	const op = "querylog.Render"

	out.Grow(len(sql) + len(args)*16)

	for i := 0; i < len(sql); {
		switch {
		case sql[i] == '\'':
			next := consumeSingleQuoted(sql, i, &out)
			i = next

		case sql[i] == '"':
			next := consumeDoubleQuoted(sql, i, &out)
			i = next

		case strings.HasPrefix(sql[i:], "--"):
			next := consumeLineComment(sql, i, &out)
			i = next

		case strings.HasPrefix(sql[i:], "/*"):
			next := consumeBlockComment(sql, i, &out)
			i = next

		case sql[i] == '$':
			if next, ok := placeholderEnd(sql, i); ok {
				index, err := strconv.Atoi(sql[i+1 : next])
				if err != nil || index <= 0 || index > len(args) {
					return "", fmt.Errorf("%s: invalid PostgreSQL placeholder %q", op, sql[i:next])
				}

				literal, err := formatLiteral(args[index-1])
				if err != nil {
					return "", fmt.Errorf("%s: render argument $%d: %w", op, index, err)
				}

				out.WriteString(literal)
				i = next
				continue
			}

			if delimiter, ok := dollarQuoteDelimiter(sql, i); ok {
				next := consumeDollarQuoted(sql, i, delimiter, &out)
				i = next
				continue
			}

			out.WriteByte(sql[i])
			i++

		default:
			out.WriteByte(sql[i])
			i++
		}
	}

	return out.String(), nil
}

// placeholderEnd reports whether sql[start:] begins with a positional
// placeholder $N (N starting at 1) and returns the index after that token.
func placeholderEnd(sql string, start int) (int, bool) {
	if start+1 >= len(sql) || sql[start+1] < '1' || sql[start+1] > '9' {
		return start, false
	}

	end := start + 2
	for end < len(sql) && sql[end] >= '0' && sql[end] <= '9' {
		end++
	}

	return end, true
}

// dollarQuoteDelimiter returns the opening delimiter for a PostgreSQL
// dollar-quoted string starting at sql[start], such as $$ or $tag$.
func dollarQuoteDelimiter(sql string, start int) (string, bool) {
	if start+1 >= len(sql) {
		return "", false
	}

	if sql[start+1] == '$' {
		return "$$", true
	}

	if !isDollarTagStart(sql[start+1]) {
		return "", false
	}

	for i := start + 2; i < len(sql); i++ {
		if sql[i] == '$' {
			return sql[start : i+1], true
		}

		if !isDollarTagPart(sql[i]) {
			return "", false
		}
	}

	return "", false
}

func isDollarTagStart(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isDollarTagPart(b byte) bool {
	return isDollarTagStart(b) || b >= '0' && b <= '9'
}

// consumeSingleQuoted copies a single-quoted literal, including escaped quotes
// (”) and a conservative treatment of backslash escapes for E'...' forms.
func consumeSingleQuoted(sql string, start int, out *strings.Builder) int {
	out.WriteByte(sql[start])
	for i := start + 1; i < len(sql); i++ {
		out.WriteByte(sql[i])
		// Treat backslash escapes conservatively for E'...' strings. Doing so
		// also avoids accidentally rendering placeholder-like text inside a
		// quoted literal when logging.
		if sql[i] == '\\' && i+1 < len(sql) {
			out.WriteByte(sql[i+1])
			i++
			continue
		}

		if sql[i] != '\'' {
			continue
		}

		if i+1 < len(sql) && sql[i+1] == '\'' {
			out.WriteByte(sql[i+1])
			i++
			continue
		}

		return i + 1
	}

	return len(sql)
}

// consumeDoubleQuoted copies a double-quoted SQL identifier, including "".
func consumeDoubleQuoted(sql string, start int, out *strings.Builder) int {
	out.WriteByte(sql[start])
	for i := start + 1; i < len(sql); i++ {
		out.WriteByte(sql[i])
		if sql[i] != '"' {
			continue
		}

		if i+1 < len(sql) && sql[i+1] == '"' {
			out.WriteByte(sql[i+1])
			i++
			continue
		}

		return i + 1
	}

	return len(sql)
}

// consumeLineComment copies a -- comment through the terminating newline.
func consumeLineComment(sql string, start int, out *strings.Builder) int {
	for i := start; i < len(sql); i++ {
		out.WriteByte(sql[i])
		if sql[i] == '\n' {
			return i + 1
		}
	}

	return len(sql)
}

// consumeBlockComment copies a /* ... */ comment, including nested blocks.
func consumeBlockComment(sql string, start int, out *strings.Builder) int {
	depth := 0
	for i := start; i < len(sql); {
		switch {
		case strings.HasPrefix(sql[i:], "/*"):
			out.WriteString("/*")
			depth++
			i += 2

		case strings.HasPrefix(sql[i:], "*/"):
			out.WriteString("*/")
			depth--
			i += 2
			if depth == 0 {
				return i
			}

		default:
			out.WriteByte(sql[i])
			i++
		}
	}

	return len(sql)
}

// consumeDollarQuoted copies a dollar-quoted string from its opening delimiter
// through the matching closer.
func consumeDollarQuoted(sql string, start int, delimiter string, out *strings.Builder) int {
	out.WriteString(delimiter)
	bodyStart := start + len(delimiter)
	endOffset := strings.Index(sql[bodyStart:], delimiter)
	if endOffset < 0 {
		out.WriteString(sql[bodyStart:])
		return len(sql)
	}

	end := bodyStart + endOffset
	out.WriteString(sql[bodyStart:end])
	out.WriteString(delimiter)
	return end + len(delimiter)
}

// formatLiteral converts a bound Go value into a PostgreSQL literal suitable
// only for log display (quoted strings, NULL, TRUE/FALSE, ARRAY[...], etc.).
func formatLiteral(value any) (string, error) {
	const op = "querylog.formatLiteral"

	if value == nil {
		return "NULL", nil
	}

	// Typed nil pointers (e.g. (*uuid.UUID)(nil)) are not equal to untyped nil,
	// but still must render as NULL. Check before driver.Valuer / Stringer:
	// those interfaces are often implemented with value receivers, and calling
	// Value/String on a nil pointer panics.
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Interface:
		if rv.IsNil() {
			return "NULL", nil
		}
	}

	switch v := value.(type) {
	case time.Time:
		return quoteString(v.Format(time.RFC3339Nano)) + "::timestamptz", nil

	case []byte:
		return byteaLiteral(v), nil

	case json.RawMessage:
		return quoteString(string(v)) + "::jsonb", nil

	case driver.Valuer:
		resolved, err := v.Value()
		if err != nil {
			return "", fmt.Errorf("%s: driver value: %w", op, err)
		}

		return formatLiteral(resolved)

	case fmt.Stringer:
		return quoteString(v.String()), nil
	}

	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "NULL", nil
		}

		rv = rv.Elem()
	}

	switch rv.Kind() {

	case reflect.String:
		return quoteString(rv.String()), nil

	case reflect.Bool:
		if rv.Bool() {
			return "TRUE", nil
		}

		return "FALSE", nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10), nil

	case reflect.Float32, reflect.Float64:
		return formatFloat(rv.Float(), rv.Type().Bits()), nil

	case reflect.Slice, reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			buf := make([]byte, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				buf[i] = byte(rv.Index(i).Uint())
			}

			return byteaLiteral(buf), nil
		}

		items := make([]string, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item, err := formatLiteral(rv.Index(i).Interface())
			if err != nil {
				return "", err
			}

			items = append(items, item)
		}

		return "ARRAY[" + strings.Join(items, ", ") + "]", nil

	case reflect.Map, reflect.Struct:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("%s: json encode %T: %w", op, value, err)
		}

		return quoteString(string(encoded)) + "::jsonb", nil

	default:
		return "", fmt.Errorf("%s: unsupported argument type %T", op, value)
	}
}

func quoteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func byteaLiteral(value []byte) string {
	return "decode('" + hex.EncodeToString(value) + "', 'hex')"
}

func formatFloat(value float64, bits int) string {
	switch {
	case math.IsNaN(value):
		return "'NaN'::float8"

	case math.IsInf(value, 1):
		return "'Infinity'::float8"

	case math.IsInf(value, -1):
		return "'-Infinity'::float8"

	default:
		return strconv.FormatFloat(value, 'g', -1, bits)
	}
}
