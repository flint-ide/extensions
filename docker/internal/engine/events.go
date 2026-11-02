package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Watch follows the engine's container events (GET /events, streamed) and
// calls onChange, debounced, until ctx ends. It returns when the stream ends;
// callers reconnect if they still want it.
func (d *Docker) Watch(ctx context.Context, debounce time.Duration, onChange func()) error {
	filters, _ := json.Marshal(map[string][]string{"type": {"container"}})
	resp, err := d.request(ctx, http.MethodGet, "/events", url.Values{"filters": {string(filters)}}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var timer *time.Timer
	dec := json.NewDecoder(resp.Body)
	for {
		var ev struct {
			Action string `json:"Action"`
		}
		if err := dec.Decode(&ev); err != nil {
			if timer != nil && timer.Stop() && ctx.Err() == nil {
				onChange() // the stream ended (daemon restart): report what was pending
			}
			if ctx.Err() != nil || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		// exec_create/exec_start fire constantly for health checks; they change nothing shown.
		if strings.HasPrefix(ev.Action, "exec_") || ev.Action == "top" {
			continue
		}
		if timer == nil {
			timer = time.AfterFunc(debounce, onChange)
		} else {
			timer.Reset(debounce)
		}
	}
}
