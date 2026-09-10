package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type LiveSession struct {
	ID         string    `json:"live_session_id"`
	CaptureID  string    `json:"capture_id"`
	Format     string    `json:"format"`
	Active     bool      `json:"active"`
	Cursor     int       `json:"cursor"`
	Updated    time.Time `json:"updated_at"`
	Generation int       `json:"generation"`
	keys       []string
	hashes     map[string]string
	events     []string
	entries    map[string]Entry
}
type LiveManager struct {
	mu             sync.Mutex
	charles        *Charles
	store          *Store
	sessions       map[string]*LiveSession
	ownedRecording bool
	ttl            time.Duration
}

func NewLiveManager(c *Charles, s *Store, ttl time.Duration) *LiveManager {
	return &LiveManager{charles: c, store: s, sessions: map[string]*LiveSession{}, ttl: ttl}
}
func (l *LiveManager) activeCount() int {
	n := 0
	for _, s := range l.sessions {
		if s.Active {
			n++
		}
	}
	return n
}
func (l *LiveManager) Start(ctx context.Context, format string, reset, start bool) (LiveSession, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if format == "" {
		format = "xml"
	}
	if format != "xml" && format != "native" && format != "json" {
		return LiveSession{}, fmt.Errorf("invalid snapshot format")
	}
	if reset && l.activeCount() > 0 {
		return LiveSession{}, fmt.Errorf("stop active live sessions before clearing Charles")
	}
	was, err := l.charles.Recording(ctx)
	if err != nil {
		return LiveSession{}, err
	}
	if reset {
		if _, err = l.charles.Get(ctx, "/session/clear"); err != nil {
			return LiveSession{}, err
		}
	}
	started := !was && start
	if started {
		if err = l.charles.Record(ctx, true); err != nil {
			return LiveSession{}, err
		}
		l.ownedRecording = true
	}
	s := &LiveSession{ID: NewID("live_"), CaptureID: NewID("cap_"), Format: format, Active: true, Updated: time.Now(), hashes: map[string]string{}, entries: map[string]Entry{}}
	if err = l.snapshot(ctx, s); err != nil {
		if started && l.activeCount() == 0 {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if stopErr := l.charles.Record(cleanup, false); stopErr == nil {
				l.ownedRecording = false
			} else {
				err = fmt.Errorf("%w; recording rollback failed: %v", err, stopErr)
			}
		}
		return LiveSession{}, err
	}
	l.sessions[s.ID] = s
	return publicSession(s), nil
}
func publicSession(s *LiveSession) LiveSession {
	return LiveSession{ID: s.ID, CaptureID: s.CaptureID, Format: s.Format, Active: s.Active, Cursor: s.Cursor, Updated: s.Updated, Generation: s.Generation}
}
func (l *LiveManager) snapshot(ctx context.Context, s *LiveSession) error {
	latest, err := l.charles.Snapshot(ctx, s.Format, s.CaptureID)
	if err != nil {
		return err
	}
	reset := len(latest) < len(s.keys)
	for i := 0; i < min(len(latest), len(s.keys)); i++ {
		if latest[i].Identity() != s.keys[i] {
			reset = true
			break
		}
	}
	generation := s.Generation
	if reset {
		generation++
	}
	keys := make([]string, len(latest))
	changed := []Entry{}
	pendingHashes := map[string]string{}
	for i, e := range latest {
		keys[i] = e.Identity()
		e.ID = digest(fmt.Sprintf("%s|%d|%d|%s", s.CaptureID, generation, i, e.Identity()))[:32]
		if old, ok := s.entries[e.ID]; ok {
			e.Sequence = old.Sequence
		} else {
			e.Sequence = len(s.entries) + len(pendingHashes) + 1
		}
		e.Normalize(s.CaptureID, e.Sequence)
		b, _ := json.Marshal(e)
		h := digest(string(b))
		if s.hashes[e.ID] != h {
			changed = append(changed, e)
			pendingHashes[e.ID] = h
		}
	}
	if err = l.store.SaveCapture(ctx, Capture{ID: s.CaptureID, Source: "live:" + s.ID, Format: s.Format}, changed); err != nil {
		return err
	}
	for _, e := range changed {
		s.hashes[e.ID] = pendingHashes[e.ID]
		s.entries[e.ID] = e
		pending := false
		for _, id := range s.events[s.Cursor:] {
			if id == e.ID {
				pending = true
				break
			}
		}
		if !pending {
			s.events = append(s.events, e.ID)
		}
	}
	s.keys = keys
	s.Generation = generation
	s.Updated = time.Now()
	return nil
}
func (l *LiveManager) Read(ctx context.Context, id string, q Query, limit int, advance bool) (QueryResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	if !ok || !s.Active {
		return QueryResult{}, fmt.Errorf("active live session %q not found", id)
	}
	m, err := newMatcher(q)
	if err != nil {
		return QueryResult{}, err
	}
	if err = l.snapshot(ctx, s); err != nil {
		return QueryResult{}, err
	}
	out := QueryResult{Entries: []map[string]any{}, Total: len(s.entries)}
	cursor := s.Cursor
	for cursor < len(s.events) {
		if len(out.Entries) >= limit {
			break
		}
		e := s.entries[s.events[cursor]]
		cursor++
		if m.match(e) {
			out.Entries = append(out.Entries, Summary(e))
			out.Matched++
		}
	}
	out.Cursor = cursor
	out.HasMore = cursor < len(s.events)
	if advance {
		s.Cursor = cursor
	}
	return out, nil
}
func (l *LiveManager) Capture(ctx context.Context, id string, refresh bool) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	if !ok {
		return "", fmt.Errorf("live session %q not found", id)
	}
	if refresh && s.Active {
		if err := l.snapshot(ctx, s); err != nil {
			return "", err
		}
	}
	return s.CaptureID, nil
}
func (l *LiveManager) stop(ctx context.Context, s *LiveSession, restore bool) error {
	if !s.Active {
		return nil
	}
	if l.activeCount() == 1 && l.ownedRecording && restore {
		if err := l.charles.Record(ctx, false); err != nil {
			return err
		}
		l.ownedRecording = false
	}
	if l.activeCount() == 1 && !restore {
		l.ownedRecording = false
	}
	s.Active = false
	s.Updated = time.Now()
	return nil
}
func (l *LiveManager) Stop(ctx context.Context, id string, restore bool) (LiveSession, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	if !ok {
		return LiveSession{}, fmt.Errorf("live session %q not found", id)
	}
	var snapshotErr error
	if s.Active {
		snapshotErr = l.snapshot(ctx, s)
	}
	err := l.stop(ctx, s, restore)
	if err == nil {
		err = snapshotErr
	}
	return publicSession(s), err
}
func (l *LiveManager) Status() []LiveSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []LiveSession{}
	for _, s := range l.sessions {
		out = append(out, publicSession(s))
	}
	return out
}
func (l *LiveManager) Expire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, s := range l.sessions {
		if time.Since(s.Updated) > l.ttl {
			if err := l.stop(ctx, s, true); err != nil {
				return err
			}
			delete(l.sessions, id)
		}
	}
	return nil
}
func (l *LiveManager) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ownedRecording {
		if err := l.charles.Record(ctx, false); err != nil {
			return err
		}
		l.ownedRecording = false
	}
	for _, s := range l.sessions {
		s.Active = false
	}
	return nil
}
func (l *LiveManager) DeleteCapture(ctx context.Context, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.sessions {
		if s.CaptureID == id && s.Active {
			return fmt.Errorf("stop live session before deleting its capture")
		}
	}
	return l.store.DeleteCapture(ctx, id)
}
