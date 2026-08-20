package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/markcheno/go-scan/internal/engine"
)

// Event is one message in a job's stream.
type Event struct {
	Type     string           `json:"type"` // progress | log | done | error
	Time     time.Time        `json:"time"`
	Progress *engine.Progress `json:"progress,omitempty"`
	Message  string           `json:"message,omitempty"`
	Result   *runSummary      `json:"result,omitempty"`
}

// runSummary is the payload of a job's terminating "done" event. The rows
// themselves stay out of the stream; the UI already previews them.
type runSummary struct {
	Files    []string               `json:"files"`
	Rows     int                    `json:"rows"`
	Kept     int                    `json:"kept"`
	Total    int                    `json:"total"`
	Errors   []engine.TickerError   `json:"errors"`
	Verdicts []engine.TickerVerdict `json:"verdicts"`
	Headers  []string               `json:"headers"`
	Elapsed  string                 `json:"elapsed"`
}

// Job is one background scan.
type Job struct {
	ID     string
	cancel context.CancelFunc

	mu     sync.Mutex
	events []Event
	subs   map[chan Event]struct{}
	done   bool
}

func (j *Job) publish(ev Event) {
	ev.Time = time.Now()

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return
	}
	if ev.Type == "done" || ev.Type == "error" {
		j.done = true
	}
	j.events = append(j.events, ev)
	for sub := range j.subs {
		select {
		case sub <- ev:
		default:
			// A subscriber that cannot keep up loses intermediate progress
			// rather than stalling the run.
		}
	}
	if j.done {
		for sub := range j.subs {
			close(sub)
		}
		j.subs = map[chan Event]struct{}{}
	}
}

// subscribe returns the events so far plus a channel of everything after them.
// The channel is closed when the job finishes; a nil channel means it already
// had.
func (j *Job) subscribe() ([]Event, chan Event) {
	j.mu.Lock()
	defer j.mu.Unlock()

	backlog := make([]Event, len(j.events))
	copy(backlog, j.events)
	if j.done {
		return backlog, nil
	}
	ch := make(chan Event, 64)
	j.subs[ch] = struct{}{}
	return backlog, ch
}

func (j *Job) unsubscribe(ch chan Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.subs[ch]; ok {
		delete(j.subs, ch)
		close(ch)
	}
}

// jobStore holds the jobs started this session.
type jobStore struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func newJobStore() *jobStore {
	return &jobStore{jobs: map[string]*Job{}}
}

func (s *jobStore) create(cancel context.CancelFunc) *Job {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	job := &Job{ID: hex.EncodeToString(buf), cancel: cancel, subs: map[chan Event]struct{}{}}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return job
}

func (s *jobStore) get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *jobStore) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		job.cancel()
	}
}
