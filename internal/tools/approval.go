package tools

import (
	"context"
	"fmt"
	"sync"
)

type ApprovalDecision struct {
	ApprovalID string
	Approved   bool
}

type ApprovalStore struct {
	mu       sync.Mutex
	waiters  map[string]chan bool
	decided  map[string]bool
	decidedN int
}

func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		waiters: map[string]chan bool{},
		decided: map[string]bool{},
	}
}

func (s *ApprovalStore) Wait(ctx context.Context, approvalID string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("approval store not configured")
	}
	if approvalID == "" {
		return false, fmt.Errorf("missing approval id")
	}
	s.mu.Lock()
	if decided, ok := s.decided[approvalID]; ok {
		s.mu.Unlock()
		return decided, nil
	}
	ch := make(chan bool, 1)
	s.waiters[approvalID] = ch
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case approved := <-ch:
		return approved, nil
	}
}

func (s *ApprovalStore) Resolve(decision ApprovalDecision) bool {
	if s == nil || decision.ApprovalID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.waiters[decision.ApprovalID]; ok {
		delete(s.waiters, decision.ApprovalID)
		ch <- decision.Approved
		close(ch)
		return true
	}
	s.decided[decision.ApprovalID] = decision.Approved
	s.decidedN++
	// Best-effort bound: keep the last ~256 decisions to avoid unbounded growth.
	if s.decidedN > 256 {
		for k := range s.decided {
			delete(s.decided, k)
			break
		}
		s.decidedN = len(s.decided)
	}
	return true
}
