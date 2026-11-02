package main

import "time"

// While the panel is shown it is read again every refreshSeconds, so new
// logins show up and expiry counts down; when it is hidden nothing runs.
// Polling only re-reads ~/.aws: STS (the network) is asked only on Check
// Identity, as in Flint's built-in panel.

type poller struct {
	done chan struct{}
}

// setVisible starts or stops polling.
func (e *ext) setVisible(visible bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if visible == (e.poll != nil) {
		return
	}
	if !visible {
		close(e.poll.done)
		e.poll = nil
		return
	}
	e.poll = e.startPoll(e.settings.interval() / e.d.scale)
}

// restartPolling applies a new interval to a running poll.
func (e *ext) restartPolling() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.poll == nil {
		return
	}
	close(e.poll.done)
	e.poll = e.startPoll(e.settings.interval() / e.d.scale)
}

// startPoll runs the timer; the caller holds e.mu.
func (e *ext) startPoll(every time.Duration) *poller {
	p := &poller{done: make(chan struct{})}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-t.C:
				// Flint resolves the panel again, which reads ~/.aws.
				e.refreshView()
			}
		}
	}()
	return p
}
