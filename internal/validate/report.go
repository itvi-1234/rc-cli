package validate

import (
	"fmt"
	"strings"
)

// report accumulates the messages of one validation. A message about a file carries its path.
type report struct{ messages []string }

func (r *report) addf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// about records a message about a file or an extension.
func (r *report) about(path string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if path != "" {
		message = path + ": " + message
	}
	r.messages = append(r.messages, message)
}

// expectExactlyOne records a message unless exactly one definition was found.
func (r *report) expectExactlyOne(path string, count int, format string, args ...any) {
	if count != 1 {
		r.about(path, format+": expected exactly one definition, got %d", append(args, count)...)
	}
}

func (r *report) err() error {
	if len(r.messages) == 0 {
		return nil
	}
	return Error(r.messages)
}

// Error is the list of problems found in a profile.
type Error []string

func (e Error) Error() string {
	if len(e) == 1 {
		return e[0]
	}
	var builder strings.Builder
	builder.WriteString("profile validation failed:")
	for _, message := range e {
		builder.WriteString("\n- ")
		builder.WriteString(message)
	}
	return builder.String()
}
