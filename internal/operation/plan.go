// Package operation contains preview and reversible-operation primitives.
package operation

import "strings"

// Change is one filesystem change shown before an operation is confirmed.
type Change struct {
	Path   string
	Action string
	Detail string
}

// Plan is a preview of a guarded operation.
type Plan struct {
	Operation string
	Changes   []Change
	Warnings  []string
}

// String renders a compact, human-readable preview.
func (p Plan) String() string {
	var b strings.Builder
	if p.Operation != "" {
		b.WriteString(p.Operation)
		b.WriteByte('\n')
	}
	for _, c := range p.Changes {
		b.WriteString(c.Action + ": " + c.Path)
		if c.Detail != "" {
			b.WriteString(" (" + c.Detail + ")")
		}
		b.WriteByte('\n')
	}
	for _, warning := range p.Warnings {
		b.WriteString("warning: " + warning + "\n")
	}
	return b.String()
}
