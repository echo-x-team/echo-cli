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
	return p.allow(false)
}

func (p Policy) AllowWrite() Decision {
	return p.allow(true)
}

func (p Policy) allow(mutating bool) Decision {
	if p.ApprovalPolicy == "auto-deny" {
		return Decision{Allowed: false, Reason: "auto-deny policy"}
	}

	if p.ApprovalPolicy == "never" {
		return Decision{Allowed: true}
	}

	if mutating && p.SandboxMode == "read-only" {
		return Decision{Allowed: false, Reason: "sandbox read-only", RequiresApproval: true}
	}

	if mutating && (p.ApprovalPolicy == "untrusted" || p.ApprovalPolicy == "on-request") {
		return Decision{Allowed: false, Reason: "requires approval", RequiresApproval: true}
	}

	return Decision{Allowed: true}
}
