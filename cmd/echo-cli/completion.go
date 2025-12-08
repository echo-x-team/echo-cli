package main

import "fmt"

func completionMain(args []string) {
	shell := "bash"
	if len(args) > 0 && args[0] != "" {
		shell = args[0]
	}
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	default:
		log.Fatalf("unsupported shell: %s (use bash or zsh)", shell)
	}
}

const bashCompletion = `
_echo_cli_completions()
{
    local cur prev
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "exec completion resume review login logout apply sandbox execpolicy mcp mcp-server cloud responses-proxy stdio-to-uds features" -- "$cur") )
        return 0
    fi

    case "${COMP_WORDS[1]}" in
        completion)
            COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") )
            return 0
            ;;
        exec)
            COMPREPLY=( $(compgen -W "--config --model --m --provider --cd --prompt --session --resume-last --list-sessions --auto-approve --auto-deny --approval-mode --ask-for-approval --sandbox --s --full-auto --dangerously-bypass-approvals-and-sandbox --yolo --run --apply-patch --add-dir --attach --image --timeout --retries --profile --oss --local-provider --output-schema --color --json --output-last-message --c --skip-git-repo-check" -- "$cur") )
            ;;
        *)
            COMPREPLY=( $(compgen -W "--config --model --m --provider --reasoning-effort --cd --C --prompt --auto-approve --auto-deny --approval-mode --ask-for-approval --sandbox --s --full-auto --dangerously-bypass-approvals-and-sandbox --yolo --profile --oss --local-provider --search --add-dir --attach --image --c --timeout --retries" -- "$cur") )
            ;;
    esac
}
complete -F _echo_cli_completions echo-cli
`

const zshCompletion = `
#compdef echo-cli
_echo_cli() {
    local -a subcmds
    subcmds=('exec:run non-interactive exec mode' 'completion:print shell completions' 'resume:resume a saved session' 'review:run review (not yet implemented)' 'login:auth stub' 'logout:auth stub' 'apply:apply diff' 'sandbox:sandbox helpers' 'execpolicy:policy helpers' 'mcp:MCP helpers' 'mcp-server:MCP server' 'cloud:cloud tasks' 'responses-proxy:responses proxy' 'stdio-to-uds:stdio bridge' 'features:list feature flags')
    if (( CURRENT == 2 )); then
        _describe 'command' subcmds
        return
    fi
    case "$words[2]" in
        completion)
            _values 'shell' bash zsh
            ;;
        exec)
            _arguments \
                '--config[Path to config file]' \
                '--model[Model override]' \
                '--m[Alias for --model]' \
                '--provider[Model provider override]' \
                '--cd[Working directory to display]' \
                '--prompt[Prompt text]' \
                '--reasoning-effort[Reasoning effort hint]' \
                '--session[Session id to resume]' \
                '--resume-last[Resume most recent session]' \
                '--list-sessions[List saved session ids]' \
                '--auto-approve[Auto approve approvals]' \
                '--auto-deny[Auto deny privileged actions]' \
                '--approval-mode[Override approval policy]' \
                '--ask-for-approval[When to request approval]' \
                '--sandbox[Sandbox mode]' \
                '--full-auto[Enable sandboxed automatic execution]' \
                '--dangerously-bypass-approvals-and-sandbox[Disable sandbox and approvals]' \
                '--yolo[Alias for dangerously bypass flags]' \
                '--run[Command to run after reply]' \
                '--apply-patch[Patch file to apply]' \
                '--profile[Config profile]' \
                '--oss[Use OSS provider]' \
                '--local-provider[Which OSS provider to use]' \
                '--output-schema[Schema file for structured output]' \
                '--color[Color output]' \
                '--json[Emit JSON events]' \
                '--output-last-message[Write last message to a file]' \
                '--add-dir[Additional workspace root]' \
                '--attach[Attach a file into context]' \
                '--image[Attach an image into context]' \
                '--c[Config key=value override]' \
                '--timeout[Request timeout seconds]' \
                '--retries[Retry count on request failure]' \
                '--skip-git-repo-check[Skip git repo validation]'
            ;;
        *)
            _arguments \
                '--config[Path to config file]' \
                '--model[Model override]' \
                '--m[Alias for --model]' \
                '--provider[Model provider override]' \
                '--reasoning-effort[Reasoning effort hint]' \
                '--cd[Working directory to display]' \
                '--C[Alias for --cd]' \
                '--prompt[Initial prompt]' \
                '--auto-approve[Auto approve approvals]' \
                '--auto-deny[Auto deny privileged actions]' \
                '--approval-mode[Override approval policy]' \
                '--ask-for-approval[When to request approval]' \
                '--sandbox[Sandbox mode]' \
                '--s[Alias for --sandbox]' \
                '--full-auto[Enable sandboxed automatic execution]' \
                '--dangerously-bypass-approvals-and-sandbox[Disable sandbox and approvals]' \
                '--yolo[Alias for bypass flag]' \
                '--profile[Config profile]' \
                '--oss[Use OSS provider]' \
                '--local-provider[Which OSS provider to use]' \
                '--search[Enable web search]' \
                '--add-dir[Additional workspace root]' \
                '--attach[Attach a file into context]' \
                '--image[Attach an image into context]' \
                '--c[Config key=value override]' \
                '--timeout[Request timeout seconds]' \
                '--retries[Retry count on request failure]'
            ;;
    esac
}
_echo_cli "$@"
`
