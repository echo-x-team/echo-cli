package repl

import tuirender "echo-cli/internal/tui/render"

// HistoryCell is an append-only render block for terminal output.
// Cells are the unit of composition: each EQ event maps to one or more cells.
type HistoryCell interface {
	// ID is an optional stable identifier for the cell (e.g., tool call id).
	// Renderers may use it to correlate updates; empty means "append-only".
	ID() string
	// Render returns styled lines for the given terminal width.
	Render(width int) []tuirender.Line
}
