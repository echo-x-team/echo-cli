package main

import (
	"flag"
	"strings"
)

// interactiveArgs captures flags shared by interactive entrypoints (echo-cli, resume).
type interactiveArgs struct {
	cfgPath         string
	modelOverride   string
	workdir         string
	prompt          string
	imagePaths      csvSlice
	configProfile   string
	oss             bool
	localProvider   string
	search          bool
	configOverrides stringSlice
	resumePicker    bool
	resumeLast      bool
	resumeSessionID string
	resumeShowAll   bool
	copyableOutput  bool
}

func newInteractiveFlagSet(name string) (*flag.FlagSet, *interactiveArgs) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	args := &interactiveArgs{}

	fs.StringVar(&args.cfgPath, "config", "", "Path to config file (default ~/.echo/config.toml)")
	fs.StringVar(&args.modelOverride, "model", "", "Model override")
	fs.StringVar(&args.modelOverride, "m", "", "Alias for --model")
	fs.StringVar(&args.workdir, "cd", "", "Working directory to display")
	fs.StringVar(&args.workdir, "C", "", "Alias for --cd")
	fs.StringVar(&args.prompt, "prompt", "", "Initial prompt")
	fs.StringVar(&args.configProfile, "profile", "", "Config profile to use")
	fs.StringVar(&args.configProfile, "p", "", "Alias for --profile")
	fs.BoolVar(&args.oss, "oss", false, "Use open-source/local provider")
	fs.StringVar(&args.localProvider, "local-provider", "", "Local OSS provider (lmstudio|ollama)")
	fs.Var(&args.imagePaths, "image", "Attach an image into initial context (comma separated or repeatable)")
	fs.Var(&args.imagePaths, "i", "Alias for --image")
	fs.BoolVar(&args.search, "search", false, "Enable web search feature flag")
	fs.Var(&args.configOverrides, "c", "Override config value key=value (repeatable)")
	fs.BoolVar(&args.copyableOutput, "copyable-output", true, "Disable alt screen to allow mouse selection/copy")

	return fs, args
}

func (i *interactiveArgs) finalizePrompt(fs *flag.FlagSet) {
	if i.prompt == "" && fs.NArg() > 0 {
		i.prompt = strings.Join(fs.Args(), " ")
	}
}
