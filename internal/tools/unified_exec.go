package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

const (
	defaultUnifiedExecYield      = 5 * time.Second
	defaultUnifiedExecMaxOutByte = 64 * 1024
	unifiedExecOutputMaxBytes    = 1 << 20 // 1 MiB
	maxUnifiedExecSessions       = 64
)

type UnifiedExecManager struct {
	mu       sync.Mutex
	sessions map[string]*unifiedExecSession
}

type unifiedExecSession struct {
	id string

	cmd  *exec.Cmd
	ptmx *os.File

	notify chan struct{}
	done   chan struct{}
	once   sync.Once

	mu        sync.Mutex
	output    byteRing
	delivered int64
	exitCode  *int
	exitErr   error
	lastUsed  time.Time
}

type byteRing struct {
	base int64
	buf  []byte
	max  int
}

func (r *byteRing) end() int64 { return r.base + int64(len(r.buf)) }

func (r *byteRing) append(p []byte) {
	if r.max <= 0 {
		r.max = unifiedExecOutputMaxBytes
	}
	if len(p) >= r.max {
		r.base += int64(len(p) - r.max)
		r.buf = append(r.buf[:0], p[len(p)-r.max:]...)
		return
	}
	if len(r.buf)+len(p) > r.max {
		drop := len(r.buf) + len(p) - r.max
		if drop > len(r.buf) {
			drop = len(r.buf)
		}
		r.base += int64(drop)
		r.buf = append(r.buf[:0], r.buf[drop:]...)
	}
	r.buf = append(r.buf, p...)
}

func (r *byteRing) slice(from int64) []byte {
	if from < r.base {
		from = r.base
	}
	offset := int(from - r.base)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(r.buf) {
		return nil
	}
	return r.buf[offset:]
}

type ExecCommandSpec struct {
	Command        string
	Workdir        string
	BaseEnv        []string
	YieldTime      time.Duration
	MaxOutputBytes int
}

type ExecCommandResult struct {
	Output    string
	SessionID string
	ExitCode  *int
}

func NewUnifiedExecManager() *UnifiedExecManager {
	return &UnifiedExecManager{sessions: map[string]*unifiedExecSession{}}
}

func (m *UnifiedExecManager) ExecCommand(ctx context.Context, spec ExecCommandSpec) (ExecCommandResult, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return ExecCommandResult{}, fmt.Errorf("empty command")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	yield := spec.YieldTime
	if yield <= 0 {
		yield = defaultUnifiedExecYield
	}
	maxOut := spec.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = defaultUnifiedExecMaxOutByte
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", spec.Command)
	if strings.TrimSpace(spec.Workdir) != "" {
		cmd.Dir = spec.Workdir
	}
	cmd.Env = withUnifiedExecEnv(spec.BaseEnv)

	sess := &unifiedExecSession{
		id:       uuid.NewString(),
		cmd:      cmd,
		notify:   make(chan struct{}, 1),
		done:     make(chan struct{}),
		lastUsed: time.Now(),
	}
	sess.output.max = unifiedExecOutputMaxBytes

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return ExecCommandResult{}, fmt.Errorf("start pty: %w", err)
	}
	sess.ptmx = ptmx

	m.mu.Lock()
	m.pruneSessionsLocked()
	if len(m.sessions) >= maxUnifiedExecSessions {
		m.mu.Unlock()
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		return ExecCommandResult{}, fmt.Errorf("too many active exec sessions")
	}
	m.sessions[sess.id] = sess
	m.mu.Unlock()

	go sess.readLoop()
	go sess.waitLoop()

	out, exitCode := sess.waitAndCollect(ctx, yield, maxOut)
	if exitCode != nil {
		m.deleteSession(sess.id)
		return ExecCommandResult{Output: out, ExitCode: exitCode}, sess.exitErr
	}
	return ExecCommandResult{Output: out, SessionID: sess.id}, nil
}

type WriteStdinSpec struct {
	SessionID      string
	Chars          string
	YieldTime      time.Duration
	MaxOutputBytes int
}

type WriteStdinResult struct {
	Output    string
	SessionID string
	ExitCode  *int
}

func (m *UnifiedExecManager) WriteStdin(ctx context.Context, spec WriteStdinSpec) (WriteStdinResult, error) {
	if strings.TrimSpace(spec.SessionID) == "" {
		return WriteStdinResult{}, fmt.Errorf("missing session_id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	yield := spec.YieldTime
	if yield <= 0 {
		yield = defaultUnifiedExecYield
	}
	maxOut := spec.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = defaultUnifiedExecMaxOutByte
	}

	sess, ok := m.getSession(spec.SessionID)
	if !ok {
		return WriteStdinResult{}, fmt.Errorf("unknown session id: %s", spec.SessionID)
	}
	sess.touch()

	if spec.Chars != "" {
		if _, err := io.WriteString(sess.ptmx, spec.Chars); err != nil {
			// If the process already exited, fall through to collect output and return exit.
		}
	}

	out, exitCode := sess.waitAndCollect(ctx, yield, maxOut)
	if exitCode != nil {
		m.deleteSession(sess.id)
		return WriteStdinResult{Output: out, ExitCode: exitCode}, sess.exitErr
	}
	return WriteStdinResult{Output: out, SessionID: sess.id}, nil
}

func (m *UnifiedExecManager) getSession(id string) (*unifiedExecSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *UnifiedExecManager) deleteSession(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.close()
}

func (m *UnifiedExecManager) pruneSessionsLocked() {
	if len(m.sessions) < maxUnifiedExecSessions {
		return
	}
	// Prefer removing finished sessions first.
	for id, s := range m.sessions {
		if s.isDone() {
			delete(m.sessions, id)
			s.close()
		}
		if len(m.sessions) < maxUnifiedExecSessions {
			return
		}
	}
	// Still full: evict the least recently used session.
	var oldestID string
	var oldest time.Time
	for id, s := range m.sessions {
		s.mu.Lock()
		t := s.lastUsed
		s.mu.Unlock()
		if oldestID == "" || t.Before(oldest) {
			oldestID = id
			oldest = t
		}
	}
	if oldestID != "" {
		s := m.sessions[oldestID]
		delete(m.sessions, oldestID)
		if s != nil {
			s.close()
		}
	}
}

func (s *unifiedExecSession) close() {
	s.once.Do(func() {
		if s.ptmx != nil {
			_ = s.ptmx.Close()
		}
		close(s.done)
	})
}

func (s *unifiedExecSession) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *unifiedExecSession) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *unifiedExecSession) notifyOutput() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *unifiedExecSession) readLoop() {
	tmp := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(tmp)
		if n > 0 {
			chunk := append([]byte(nil), tmp[:n]...)
			s.mu.Lock()
			beforeBase := s.output.base
			s.output.append(chunk)
			if s.delivered < s.output.base && beforeBase != s.output.base {
				s.delivered = s.output.base
			}
			s.mu.Unlock()
			s.notifyOutput()
		}
		if err != nil {
			return
		}
	}
}

func (s *unifiedExecSession) waitLoop() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	s.mu.Lock()
	s.exitCode = &code
	s.exitErr = err
	s.mu.Unlock()
	s.notifyOutput()
	s.close()
}

func (s *unifiedExecSession) waitAndCollect(ctx context.Context, yield time.Duration, maxOut int) (string, *int) {
	timer := time.NewTimer(yield)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return "", nil
	case <-s.done:
	case <-timer.C:
	case <-s.notify:
		// Debounce a bit to coalesce bursts.
		short := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			short.Stop()
			return "", nil
		case <-s.done:
		case <-short.C:
		}
	}

	out := s.takeOutput(maxOut)
	s.mu.Lock()
	exit := s.exitCode
	s.mu.Unlock()
	return out, exit
}

func (s *unifiedExecSession) takeOutput(maxOut int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.output.slice(s.delivered)
	if len(p) == 0 {
		return ""
	}
	if maxOut > 0 && len(p) > maxOut {
		p = p[:maxOut]
		s.delivered += int64(len(p))
		return string(p)
	}
	s.delivered = s.output.end()
	return string(p)
}

func withUnifiedExecEnv(base []string) []string {
	env := append([]string{}, base...)
	env = setEnv(env, "NO_COLOR", "1")
	env = setEnv(env, "TERM", "dumb")
	env = setEnv(env, "PAGER", "cat")
	env = setEnv(env, "GIT_PAGER", "cat")
	env = setEnv(env, "LANG", "C")
	env = setEnv(env, "LC_ALL", "C")
	env = setEnv(env, "GIT_TERMINAL_PROMPT", "0")
	return env
}
