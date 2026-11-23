package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const uploadSessionHeader = "X-File-Upload-Session"

// uploadSessionTTL is how long an incomplete upload may keep exclusive claim on a path.
// Successful chunks refresh the lease; abandoned sessions expire and can be replaced.
var uploadSessionTTL = 30 * time.Minute

// uploadSessionTakeoverGrace is how long a session must have been idle (no request
// in flight) before an explicit overwrite from another session may replace it.
// Active uploads send chunks back to back, so their gaps are far shorter than this.
var uploadSessionTakeoverGrace = 5 * time.Second

var errUploadSessionConflict = errors.New("conflicting upload session")

type uploadSessionEntry struct {
	id       string
	lastSeen time.Time
	inFlight int // requests of this session currently being handled
}

type uploadSessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]uploadSessionEntry // realPath -> active session
}

var activeUploadSessions uploadSessionRegistry

// acquire claims realPath for sessionID and marks one request of that session as in
// flight. Every successful acquire must be paired with a call to done.
//
// When takeover is true (an explicit overwrite starting a new upload), a claim held by
// another session is replaced if that session has no request in flight and has been
// idle for at least uploadSessionTakeoverGrace, i.e. it looks abandoned rather than
// actively uploading.
func (r *uploadSessionRegistry) acquire(realPath, sessionID string, takeover bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = make(map[string]uploadSessionEntry)
	}
	now := time.Now()
	entry := uploadSessionEntry{id: sessionID}
	if active, ok := r.sessions[realPath]; ok {
		idle := now.Sub(active.lastSeen)
		switch {
		case active.id == sessionID:
			entry = active
		case idle > uploadSessionTTL:
			// Expired lease: replace it.
		case takeover && active.inFlight == 0 && idle >= uploadSessionTakeoverGrace:
			// Abandoned session and the client asked to overwrite: replace it.
		default:
			return fmt.Errorf("%w for %q", errUploadSessionConflict, realPath)
		}
	}
	entry.lastSeen = now
	entry.inFlight++
	r.sessions[realPath] = entry
	return nil
}

// done marks one request of sessionID as finished and refreshes the lease, so the
// idle time is measured from the end of the last request rather than its start.
// It is a no-op if the claim was already released or taken over.
func (r *uploadSessionRegistry) done(realPath, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active, ok := r.sessions[realPath]; ok && active.id == sessionID {
		if active.inFlight > 0 {
			active.inFlight--
		}
		active.lastSeen = time.Now()
		r.sessions[realPath] = active
	}
}

func (r *uploadSessionRegistry) release(realPath, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active, ok := r.sessions[realPath]; ok && active.id == sessionID {
		delete(r.sessions, realPath)
	}
}

// idleForTest backdates a path's session lease by d (for unit tests).
func (r *uploadSessionRegistry) idleForTest(realPath string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active, ok := r.sessions[realPath]; ok {
		active.lastSeen = time.Now().Add(-d)
		r.sessions[realPath] = active
	}
}

// expireForTest marks a path's session as expired without deleting it (for unit tests).
func (r *uploadSessionRegistry) expireForTest(realPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active, ok := r.sessions[realPath]; ok {
		active.lastSeen = time.Now().Add(-uploadSessionTTL - time.Second)
		r.sessions[realPath] = active
	}
}

func isUploadSessionConflict(err error) bool {
	return errors.Is(err, errUploadSessionConflict)
}

// parseUploadSession reads X-File-Upload-Session. The value must be a short safe token.
func parseUploadSession(r *http.Request) (string, error) {
	s := strings.TrimSpace(r.Header.Get(uploadSessionHeader))
	if s == "" {
		return "", fmt.Errorf("missing %s", uploadSessionHeader)
	}
	if len(s) > 128 {
		return "", fmt.Errorf("invalid upload session")
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return "", fmt.Errorf("invalid upload session")
	}
	return s, nil
}
