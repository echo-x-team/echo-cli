package features

// Stage mirrors the lifecycle buckets used by echo-rs for feature flags.
type Stage string

const (
	StageStable       Stage = "stable"
	StageBeta         Stage = "beta"
	StageExperimental Stage = "experimental"
	StageDeprecated   Stage = "deprecated"
	StageRemoved      Stage = "removed"
)

// Spec describes a feature flag exposed by the CLI.
type Spec struct {
	Key            string
	Stage          Stage
	DefaultEnabled bool
}

// Specs mirrors the feature surface of echo-rs.
var Specs = []Spec{
	{Key: "undo", Stage: StageStable, DefaultEnabled: true},
	{Key: "view_image_tool", Stage: StageStable, DefaultEnabled: true},
	{Key: "shell_tool", Stage: StageStable, DefaultEnabled: true},
	{Key: "unified_exec", Stage: StageExperimental, DefaultEnabled: false},
	{Key: "rmcp_client", Stage: StageExperimental, DefaultEnabled: false},
	{Key: "apply_patch_freeform", Stage: StageBeta, DefaultEnabled: false},
	{Key: "web_search_request", Stage: StageStable, DefaultEnabled: false},
	{Key: "remote_compaction", Stage: StageExperimental, DefaultEnabled: true},
	{Key: "parallel", Stage: StageExperimental, DefaultEnabled: false},
	{Key: "warnings", Stage: StageExperimental, DefaultEnabled: false},
	{Key: "skills", Stage: StageExperimental, DefaultEnabled: false},
}

var known = func() map[string]Spec {
	m := make(map[string]Spec, len(Specs))
	for _, spec := range Specs {
		m[spec.Key] = spec
	}
	return m
}()

// IsKnown reports whether the feature key is recognized.
func IsKnown(key string) bool {
	_, ok := known[key]
	return ok
}

// StageFor returns the lifecycle stage for a feature, defaulting to experimental.
func StageFor(key string) Stage {
	if spec, ok := known[key]; ok {
		return spec.Stage
	}
	return StageExperimental
}

// DefaultEnabled reports the default value for the given feature key.
func DefaultEnabled(key string) bool {
	if spec, ok := known[key]; ok {
		return spec.DefaultEnabled
	}
	return false
}
