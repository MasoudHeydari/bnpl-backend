package slog

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

const (
	ansiReset  = "\x1b[0m"
	ansiKey    = "\x1b[36m" // Cyan.
	ansiString = "\x1b[32m" // Green.
	ansiNumber = "\x1b[33m" // Yellow.
	ansiBool   = "\x1b[35m" // Magenta.
	ansiNull   = "\x1b[90m" // Gray.

	jsonIndent = "  "
)

// prettyJSONWriter formats newline-delimited JSON records for humans.
//
// Each complete record from slog.JSONHandler is indented and colorized with
// ANSI escape codes. Incomplete trailing chunks are buffered until the next
// Write call.
type prettyJSONWriter struct {
	mu  sync.Mutex
	out io.Writer
	buf bytes.Buffer
}

// newPrettyJSONWriter creates a writer that pretty-prints JSON log records.
func newPrettyJSONWriter(out io.Writer) *prettyJSONWriter {
	return &prettyJSONWriter{out: out}
}

// Write receives newline-delimited JSON records from slog.JSONHandler.
func (w *prettyJSONWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return writeBufferedJSONRecords(&w.buf, p, func(record []byte) error {
		return writePrettyJSONRecord(w.out, record)
	})
}

// splitJSONWriter dual-writes each JSON log record:
//
//   - compact, machine-readable JSON to machine (e.g. LOG_FILE)
//   - pretty, human-oriented JSON to human (e.g. stdout)
//
// Each destination receives the record once.
type splitJSONWriter struct {
	mu      sync.Mutex
	human   io.Writer
	machine io.Writer
	buf     bytes.Buffer
}

// newSplitJSONWriter creates a dual-sink JSON writer.
func newSplitJSONWriter(human, machine io.Writer) *splitJSONWriter {
	return &splitJSONWriter{
		human:   human,
		machine: machine,
	}
}

// Write fans out complete JSON records to machine (compact) and human (pretty).
func (w *splitJSONWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return writeBufferedJSONRecords(&w.buf, p, func(record []byte) error {
		if _, err := w.machine.Write(record); err != nil {
			return err
		}

		if _, err := io.WriteString(w.machine, "\n"); err != nil {
			return err
		}

		return writePrettyJSONRecord(w.human, record)
	})
}

func writeBufferedJSONRecords(
	buf *bytes.Buffer,
	p []byte,
	writeRecord func([]byte) error,
) (int, error) {
	inputLength := len(p)

	// bytes.Buffer.Write never returns an error, but keeping the check makes
	// the write flow explicit and protects this code if the buffer changes.
	if _, err := buf.Write(p); err != nil {
		return 0, err
	}

	for {
		newlineIndex := bytes.IndexByte(buf.Bytes(), '\n')
		if newlineIndex < 0 {
			break
		}

		// Copy the record because bytes.Buffer.Next returns a slice backed by
		// the buffer's internal storage.
		record := bytes.Clone(buf.Next(newlineIndex + 1))
		record = bytes.TrimSpace(record)

		if len(record) == 0 {
			continue
		}

		if err := writeRecord(record); err != nil {
			// The complete input was accepted into the internal buffer even
			// though writing the formatted output failed.
			return inputLength, err
		}
	}

	return inputLength, nil
}

func writePrettyJSONRecord(out io.Writer, record []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, record, "", jsonIndent); err != nil {
		// JSONHandler should always emit valid JSON. Falling back to the
		// original record prevents a formatting failure from losing the log.
		if _, writeErr := out.Write(record); writeErr != nil {
			return writeErr
		}

		_, writeErr := io.WriteString(out, "\n")
		return writeErr
	}

	// Expand escaped whitespace inside JSON string values so multiline
	// fields such as SQL stay human-readable in development terminals.
	// The result is display-oriented and may no longer be strict JSON.
	display := expandEscapedWhitespaceInJSONStrings(pretty.Bytes())
	if err := writeColoredJSON(out, display); err != nil {
		return err
	}

	_, err := io.WriteString(out, "\n")
	return err
}

// expandEscapedWhitespaceInJSONStrings rewrites JSON string values so escaped
// newlines and tabs become literal characters for terminal readability.
//
// Object keys are left unchanged. Quotes and backslashes inside values remain
// escaped so the surrounding pretty-printer can still scan token boundaries.
func expandEscapedWhitespaceInJSONStrings(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))

	for index := 0; index < len(data); {
		if data[index] != '"' {
			out.WriteByte(data[index])
			index++
			continue
		}

		end := findJSONStringEnd(data, index)
		if isJSONObjectKey(data, end) {
			out.Write(data[index:end])
			index = end
			continue
		}

		out.WriteByte('"')
		escaped := false
		for cursor := index + 1; cursor < end-1; cursor++ {
			current := data[cursor]
			if escaped {
				switch current {
				case 'n':
					out.WriteByte('\n')

				case 't':
					out.WriteByte('\t')

				case 'r':
					out.WriteByte('\r')

				case '"', '\\', '/':
					out.WriteByte('\\')
					out.WriteByte(current)

				default:
					out.WriteByte('\\')
					out.WriteByte(current)
				}

				escaped = false
				continue
			}

			if current == '\\' {
				escaped = true
				continue
			}

			out.WriteByte(current)
		}

		out.WriteByte('"')
		index = end
	}

	return out.Bytes()
}

// writeColoredJSON writes already-formatted JSON with ANSI syntax coloring.
//
// Coloring is performed directly on the original JSON bytes rather than by
// decoding into map[string]any. This preserves:
//
//   - the original slog attribute order;
//   - exact integer and floating-point representations;
//   - duplicate object keys, should they ever be emitted;
//   - JSON string escaping produced by slog.JSONHandler.
func writeColoredJSON(w io.Writer, data []byte) error {
	for index := 0; index < len(data); {
		switch current := data[index]; {
		case current == '"':
			end := findJSONStringEnd(data, index)

			color := ansiString
			if isJSONObjectKey(data, end) {
				color = ansiKey
			}

			if err := writeColoredToken(w, color, data[index:end]); err != nil {
				return err
			}

			index = end

		case current == '-' || isDigit(current):
			end := findJSONNumberEnd(data, index)

			if err := writeColoredToken(w, ansiNumber, data[index:end]); err != nil {
				return err
			}

			index = end

		case hasJSONLiteral(data[index:], "true"):
			if err := writeColoredToken(w, ansiBool, data[index:index+4]); err != nil {
				return err
			}

			index += 4

		case hasJSONLiteral(data[index:], "false"):
			if err := writeColoredToken(w, ansiBool, data[index:index+5]); err != nil {
				return err
			}

			index += 5

		case hasJSONLiteral(data[index:], "null"):
			if err := writeColoredToken(w, ansiNull, data[index:index+4]); err != nil {
				return err
			}

			index += 4

		default:
			if _, err := w.Write(data[index : index+1]); err != nil {
				return err
			}

			index++
		}
	}

	return nil
}

// writeColoredToken wraps a JSON token in an ANSI color sequence.
func writeColoredToken(w io.Writer, color string, token []byte) error {
	if _, err := io.WriteString(w, color); err != nil {
		return err
	}

	if _, err := w.Write(token); err != nil {
		return err
	}

	_, err := io.WriteString(w, ansiReset)
	return err
}

// findJSONStringEnd returns the index immediately after a JSON string.
//
// Escaped quotes are ignored so strings such as "value with \"quotes\"" are
// parsed correctly.
func findJSONStringEnd(data []byte, start int) int {
	escaped := false

	for index := start + 1; index < len(data); index++ {
		switch {
		case escaped:
			escaped = false

		case data[index] == '\\':
			escaped = true

		case data[index] == '"':
			return index + 1
		}
	}

	// The input comes from slog.JSONHandler and should always be valid JSON.
	// Returning the input length provides a safe fallback for malformed data.
	return len(data)
}

// isJSONObjectKey reports whether a JSON string is followed by an object-key
// separator after optional whitespace.
func isJSONObjectKey(data []byte, stringEnd int) bool {
	for index := stringEnd; index < len(data); index++ {
		switch data[index] {
		case ' ', '\t', '\r', '\n':
			continue

		default:
			return data[index] == ':'
		}
	}

	return false
}

// findJSONNumberEnd returns the index immediately after a JSON number.
func findJSONNumberEnd(data []byte, start int) int {
	index := start

	for index < len(data) {
		switch data[index] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'-', '+', '.', 'e', 'E':
			index++

		default:
			return index
		}
	}

	return index
}

// hasJSONLiteral reports whether data starts with the provided JSON literal.
func hasJSONLiteral(data []byte, literal string) bool {
	return len(data) >= len(literal) &&
		string(data[:len(literal)]) == literal
}

// isDigit reports whether value is an ASCII decimal digit.
func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
