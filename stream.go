package hotwire

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/antage/eventsource.v1"
)

//go:embed assets/*.html
var assets embed.FS

var tplEvent *template.Template

type (
	StreamAction string
	Stream       struct {
		Action StreamAction
		Target string
		Body   template.HTML
	}
	EventStream interface {
		http.Handler
		SendEvent(
			group string,
			action StreamAction,
			id string,
			tpl *template.Template,
			values interface{},
		) error
		Close(
			group string,
		)
	}
	eventStream struct {
		sync.RWMutex
		eventSources map[string]eventsource.EventSource
		sequence     uint64
	}
)

const (
	StreamActionAppend  StreamAction = "append"
	StreamActionPrepend StreamAction = "prepend"
	StreamActionRemove  StreamAction = "remove"
	StreamActionReplace StreamAction = "replace"
	StreamActionUpdate  StreamAction = "update"
)

func (s *Stream) Render() (string, error) {
	event := &bytes.Buffer{}
	if err := tplEvent.Execute(event, s); err != nil {
		return "", fmt.Errorf("error executing event template, %w", err)
	}
	return event.String(), nil
}

func NewEventStream() EventStream {
	tplEvent = template.Must(
		template.ParseFS(assets, "assets/payload.turbo-stream.html"),
	)
	return &eventStream{
		eventSources: map[string]eventsource.EventSource{},
		sequence:     1,
	}
}

func (h *eventStream) getEventSource(group string) eventsource.EventSource {
	h.RLock()
	es, ok := h.eventSources[group]
	if ok {
		h.RUnlock()
		return es
	}
	h.RUnlock()
	h.Lock()
	es = eventsource.New(nil, EventStreamHeaders)
	h.eventSources[group] = es
	h.Unlock()
	return es
}

func (h *eventStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	es := h.getEventSource(group)
	es.ServeHTTP(w, r)
}

func (h *eventStream) SendEvent(
	group string,
	action StreamAction,
	target string,
	tpl *template.Template,
	values interface{},
) error {
	body := &bytes.Buffer{}
	err := tpl.Execute(
		body,
		values,
	)
	if err != nil {
		return fmt.Errorf("error executing template, %w", err)
	}

	stream := &Stream{
		Action: action,
		Target: target,
		Body:   template.HTML(body.String()),
	}

	event, err := stream.Render()
	if err != nil {
		return fmt.Errorf("error executing template, %w", err)
	}

	n := atomic.AddUint64(&h.sequence, 1)
	h.getEventSource(group).
		SendEventMessage(
			event,
			"message",
			fmt.Sprintf("%d", n),
		)
	return nil
}

func (h *eventStream) Close(group string) {
	h.getEventSource(group).Close()
}

func EventStreamHeaders(*http.Request) [][]byte {
	return [][]byte{
		[]byte("Content-Type: text/event-stream"),
		[]byte("Cache-Control: no-cache"),
		[]byte("Connection: keep-alive"),
	}
}
