// Package forward re-sends accepted readings to public maps (gmcmap.com,
// radmon.org or compatible servers; PD-05, IMPL-34). Forwarding is
// asynchronous: a slow or failing target never delays or fails ingest. Each
// reading is tried once; the device's next upload follows within minutes.
package forward

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

// Results of a forward attempt, used as the result label of the forward counter.
const (
	ResultOK      = "ok"
	ResultError   = "error"
	ResultDropped = "dropped" // queue full
	ResultSkipped = "skipped" // the target does not take this reading
)

const (
	queueSize      = 32
	requestTimeout = 10 * time.Second
	maxBodyLog     = 200
)

// Target builds the outgoing request for one reading and judges the reply.
type Target interface {
	// Name is the value of the target label, e.g. "gmcmap".
	Name() string
	// Request returns the request to send, or nil to skip the reading.
	Request(ctx context.Context, r ingest.Reading) (*http.Request, error)
	// Check returns an error when the reply means the upload was refused.
	Check(status int, body string) error
}

// Counter counts forward attempts per target and result.
type Counter interface {
	CountForward(target, result string)
}

// Recorder wraps an ingest.Recorder and queues every recorded reading for
// each target. Start must be called before use and Close at shutdown.
type Recorder struct {
	next    ingest.Recorder
	counter Counter
	log     *slog.Logger
	client  *http.Client
	workers []*worker
	wg      sync.WaitGroup
}

type worker struct {
	target Target
	queue  chan ingest.Reading
}

// New returns a Recorder forwarding to targets. With no targets it only
// passes readings through to next.
func New(next ingest.Recorder, counter Counter, log *slog.Logger, targets ...Target) *Recorder {
	f := &Recorder{
		next: next, counter: counter, log: log,
		client: &http.Client{Timeout: requestTimeout},
	}
	for _, t := range targets {
		f.workers = append(f.workers, &worker{target: t, queue: make(chan ingest.Reading, queueSize)})
	}
	return f
}

// Start runs one sender goroutine per target until Close is called. Requests
// use ctx, so once it is cancelled the remaining queued readings fail fast.
func (f *Recorder) Start(ctx context.Context) {
	for _, w := range f.workers {
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			for r := range w.queue {
				f.send(ctx, w.target, r)
			}
		}()
	}
}

// Close stops accepting readings and waits for the senders to finish. Call it
// after the ingest server has stopped, so Record is no longer called.
func (f *Recorder) Close() {
	for _, w := range f.workers {
		close(w.queue)
	}
	f.wg.Wait()
}

// Record implements ingest.Recorder: record locally, then queue for forwarding.
func (f *Recorder) Record(r ingest.Reading) {
	f.next.Record(r)
	for _, w := range f.workers {
		select {
		case w.queue <- r:
		default:
			f.counter.CountForward(w.target.Name(), ResultDropped)
			f.log.Warn("forward queue full, reading dropped", "target", w.target.Name(), "device_id", r.DeviceID)
		}
	}
}

// Count implements ingest.Recorder.
func (f *Recorder) Count(protocol, result string) { f.next.Count(protocol, result) }

func (f *Recorder) send(ctx context.Context, t Target, r ingest.Reading) {
	req, err := t.Request(ctx, r)
	if err == nil && req == nil {
		f.counter.CountForward(t.Name(), ResultSkipped)
		return
	}
	if err == nil {
		err = f.do(req, t)
	}
	if err != nil {
		f.counter.CountForward(t.Name(), ResultError)
		f.log.Warn("forward failed", "target", t.Name(), "device_id", r.DeviceID, "error", err)
		return
	}
	f.counter.CountForward(t.Name(), ResultOK)
	f.log.Debug("forwarded", "target", t.Name(), "device_id", r.DeviceID)
}

func (f *Recorder) do(req *http.Request, t Target) error {
	resp, err := f.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err // drop the URL: it may carry a password
		}
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	if len(text) > maxBodyLog {
		text = text[:maxBodyLog]
	}
	if err := t.Check(resp.StatusCode, text); err != nil {
		return fmt.Errorf("%w (status %d, body %q)", err, resp.StatusCode, text)
	}
	return nil
}
