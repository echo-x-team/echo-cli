package policy

type Policy struct {
	SandboxMode    string
	ApprovalPolicy string
}

type Decision struct {
	Allowed          bool
	Reason           string
	RequiresApproval bool
}

func (p Policy) AllowCommand() Decision {
	if p.SandboxMode == "read-only" {
		return Decision{Allowed: false, Reason: "blocked by sandbox read-only"}
	}
	if p.ApprovalPolicy == "untrusted" || p.ApprovalPolicy == "on-request" {
		return Decision{Allowed: false, Reason: "requires approval", RequiresApproval: true}
	}
	if p.ApprovalPolicy == "auto-deny" {
		return Decision{Allowed: false, Reason: "auto-deny policy"}
	}
	if p.ApprovalPolicy == "on-failure" {
		return Decision{Allowed: true, Reason: "allow until failure"}
	}
	return Decision{Allowed: true}
}

func (p Policy) AllowWrite() Decision {
	if p.SandboxMode == "read-only" {
		return Decision{Allowed: false, Reason: "blocked by sandbox read-only"}
	}
	if p.ApprovalPolicy == "untrusted" || p.ApprovalPolicy == "on-request" {
		return Decision{Allowed: false, Reason: "requires approval", RequiresApproval: true}
	}
	if p.ApprovalPolicy == "auto-deny" {
		return Decision{Allowed: false, Reason: "auto-deny policy"}
	}
	if p.ApprovalPolicy == "on-failure" {
		return Decision{Allowed: true, Reason: "allow until failure"}
	}
	return Decision{Allowed: true}
}
