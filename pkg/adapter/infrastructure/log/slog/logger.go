package slog

import (
	"bytes"
	"encoding/json"
	"io"
	stdslog "log/slog"
	"os"
	"sync"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// Verify that Slog implements the core Logger contract at compile time.
var _ corelog.Logger = (*Slog)(nil)

// Slog adapts the standard library slog.Logger to the core logging contract.
//
// This adapter keeps the core layer independent from the concrete logging
// implementation while preserving structured logs, contextual attributes,
// configurable log levels, and configurable output formats.
type Slog struct {
	logger *stdslog.Logger
}

// New constructs a structured logger backed by the standard library slog package.
//
// When w is nil, logs are written to os.Stdout.
//
// When level is nil, the minimum log level defaults to INFO.
//
// JSONFormat normally emits compact JSON records suitable for production log
// collectors. When DEBUG logging is enabled, JSON records are automatically
// pretty-printed and colorized for easier local development.
//
// TextFormat emits the standard human-readable slog text format.
//
// Unknown formats fall back to JSON because structured JSON is generally safer
// for production log collection and processing.
func New(w io.Writer, level stdslog.Leveler, format corelog.Format) *Slog {
	if w == nil {
		w = os.Stdout
	}

	if level == nil {
		level = stdslog.LevelInfo
	}

	if level == stdslog.LevelDebug {
		w = newDebugJSONWriter(w, level)
	}

	options := &stdslog.HandlerOptions{
		Level: level,
	}

	var handler stdslog.Handler

	switch format {
	case corelog.TextFormat:
		handler = stdslog.NewTextHandler(w, options)

	case corelog.JSONFormat:
		fallthrough

	default:
		// The writer checks the configured level for every record. This means
		// pretty and colored output also responds to changes made through a
		// dynamic slog.LevelVar after the logger has been created.
		handler = stdslog.NewJSONHandler(w, options)
	}

	return &Slog{
		logger: stdslog.New(handler),
	}
}

// NewDefault constructs a production-oriented JSON logger.
//
// The default logger writes compact JSON records to stdout and only emits
// records at INFO level or higher.
func NewDefault() *Slog {
	return New(os.Stdout, stdslog.LevelInfo, corelog.JSONFormat)
}

// With returns a child logger enriched with structured attributes.
//
// The returned logger inherits all existing attributes and appends args as
// slog key-value pairs or slog.Attr values.
func (l *Slog) With(args ...any) corelog.Logger {
	return &Slog{
		logger: l.logger.With(args...),
	}
}

// Debug records diagnostic information useful during development.
func (l *Slog) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info records a normal operational event.
func (l *Slog) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn records an unexpected but non-fatal condition.
func (l *Slog) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error records a failed operation or a condition requiring attention.
func (l *Slog) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

const (
	ansiReset  = "\x1b[0m"
	ansiKey    = "\x1b[36m" // Cyan.
	ansiString = "\x1b[32m" // Green.
	ansiNumber = "\x1b[33m" // Yellow.
	ansiBool   = "\x1b[35m" // Magenta.
	ansiNull   = "\x1b[90m" // Gray.

	jsonIndent = "  "
)

// debugJSONWriter formats JSON records for development.
//
// When the configured level allows DEBUG records, each JSON record is indented
// and colorized using ANSI escape codes. At higher minimum levels, records are
// written unchanged as compact, valid JSON.
//
// The writer buffers incomplete records because an io.Writer call is not
// guaranteed to contain exactly one complete slog JSON record.
type debugJSONWriter struct {
	mu    sync.Mutex
	out   io.Writer
	level stdslog.Leveler
	buf   bytes.Buffer
}

// newDebugJSONWriter creates a JSON writer that switches between development
// and production formatting based on the currently configured log level.
func newDebugJSONWriter(out io.Writer, level stdslog.Leveler) *debugJSONWriter {
	return &debugJSONWriter{
		out:   out,
		level: level,
	}
}

// Write receives newline-delimited JSON records from slog.JSONHandler.
//
// Complete records are written immediately. An incomplete trailing record is
// retained until the next Write call.
func (w *debugJSONWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	inputLength := len(p)

	// bytes.Buffer.Write never returns an error, but keeping the check makes
	// the write flow explicit and protects this code if the buffer changes.
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}

	for {
		newlineIndex := bytes.IndexByte(w.buf.Bytes(), '\n')
		if newlineIndex < 0 {
			break
		}

		// Copy the record because bytes.Buffer.Next returns a slice backed by
		// the buffer's internal storage.
		record := bytes.Clone(w.buf.Next(newlineIndex + 1))
		record = bytes.TrimSpace(record)

		if len(record) == 0 {
			continue
		}

		if err := w.writeRecord(record); err != nil {
			// The complete input was accepted into the internal buffer even
			// though writing the formatted output failed.
			return inputLength, err
		}
	}

	return inputLength, nil
}

// writeRecord writes one complete JSON log record.
//
// DEBUG mode produces indented and colorized output. Non-DEBUG mode preserves
// the compact JSON generated by slog.JSONHandler.
func (w *debugJSONWriter) writeRecord(record []byte) error {
	if !debugLoggingEnabled(w.level) {
		if _, err := w.out.Write(record); err != nil {
			return err
		}

		_, err := io.WriteString(w.out, "\n")
		return err
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, record, "", jsonIndent); err != nil {
		// JSONHandler should always emit valid JSON. Falling back to the
		// original record prevents a formatting failure from losing the log.
		if _, writeErr := w.out.Write(record); writeErr != nil {
			return writeErr
		}

		_, writeErr := io.WriteString(w.out, "\n")
		return writeErr
	}

	// Expand escaped whitespace inside JSON string values so multiline
	// fields such as SQL stay human-readable in development terminals.
	// The result is display-oriented and may no longer be strict JSON.
	display := expandEscapedWhitespaceInJSONStrings(pretty.Bytes())
	if err := writeColoredJSON(w.out, display); err != nil {
		return err
	}

	_, err := io.WriteString(w.out, "\n")
	return err
}

// debugLoggingEnabled reports whether the configured minimum level allows
// DEBUG records.
//
// slog levels become more verbose as their numeric value decreases. Therefore,
// DEBUG is enabled when the configured minimum level is DEBUG or lower.
func debugLoggingEnabled(level stdslog.Leveler) bool {
	return level.Level() <= stdslog.LevelDebug
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
