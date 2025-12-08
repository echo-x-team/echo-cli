package tools

type ToolKind string

const (
	ToolCommand    ToolKind = "command_execution"
	ToolApplyPatch ToolKind = "file_change"
	ToolFileRead   ToolKind = "file_read"
	ToolSearch     ToolKind = "file_search"
)

type ToolRequest struct {
	ID      string
	Kind    ToolKind
	Command string
	Patch   string
	Path    string
	Query   string
}

type ToolResult struct {
	ID       string
	Kind     ToolKind
	Status   string // started|updated|completed|error
	Output   string
	Error    string
	ExitCode int
	Path     string
	Command  string
}

type ToolEvent struct {
	Type   string // approval.requested|approval.completed|item.started|item.updated|item.completed
	Result ToolResult
	Reason string
}
