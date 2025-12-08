package main

import (
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/session"
)

func resumeMain(root rootArgs, args []string) {
	fs, cli := newInteractiveFlagSet("resume")
	var sessionID string
	var resumeLast bool
	var resumeAll bool
	fs.StringVar(&sessionID, "session", "", "Session id to resume")
	fs.BoolVar(&resumeLast, "last", false, "Resume most recent session")
	fs.BoolVar(&resumeAll, "all", false, "Show all sessions (picker output only)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse resume args: %v", err)
	}

	extra := fs.Args()
	if sessionID == "" && len(extra) > 0 {
		sessionID = extra[0]
		extra = extra[1:]
	}
	if cli.prompt == "" && len(extra) > 0 {
		cli.prompt = strings.Join(extra, " ")
	}

	cli.resumeLast = resumeLast
	cli.resumeSessionID = sessionID
	cli.resumePicker = sessionID == "" && !resumeLast
	cli.resumeShowAll = resumeAll
	cli.configOverrides = stringSlice(prependOverrides(root.overrides, []string(cli.configOverrides)))

	var seed []agent.Message
	if sessionID != "" || resumeLast {
		var rec session.Record
		var err error
		if resumeLast {
			rec, err = session.Last()
		} else {
			rec, err = session.Load(sessionID)
		}
		if err != nil {
			log.Fatalf("failed to load session: %v", err)
		}
		seed = append(seed, rec.Messages...)
	}

	startInteractiveSession(cli, seed)
}
