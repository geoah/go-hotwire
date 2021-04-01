package main

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/geoah/go-hotwire"
)

//go:embed assets/*.html
var assets embed.FS

var (
	tplIndex = template.Must(template.ParseFS(
		assets,
		"assets/base.html",
		"assets/inner.message.html",
		"assets/frame.room.html",
	))
	tplMessagesNew = template.Must(template.ParseFS(
		assets,
		"assets/base.html",
		"assets/frame.messages-new.html",
	))
	tplMessageInner = template.Must(template.ParseFS(
		assets,
		"assets/inner.message.html",
	))
)

type (
	Room struct {
		sync.RWMutex
		handlers []chan<- *Message
		Messages []*Message
	}
	Message struct {
		ID      int64
		Body    string
		Created time.Time
	}
)

func (r *Room) AddMessage(body string) *Message {
	r.Lock()
	defer r.Unlock()
	m := &Message{
		ID:      time.Now().UnixNano(),
		Body:    body,
		Created: time.Now().UTC().Truncate(time.Second),
	}
	r.Messages = append(r.Messages, m)
	for _, c := range r.handlers {
		select {
		case c <- m:
		default:
			// TODO delete c?
		}
	}
	return m
}

func (r *Room) ListMessages() []*Message {
	r.RLock()
	defer r.RUnlock()
	return r.Messages
}

func (r *Room) OnAddMessage() <-chan *Message {
	r.Lock()
	defer r.Unlock()
	c := make(chan *Message)
	r.handlers = append(r.handlers, c)
	return c
}

func main() {
	c := Room{
		handlers: []chan<- *Message{},
		RWMutex:  sync.RWMutex{},
		Messages: []*Message{{
			Body:    "hello world",
			Created: time.Now().UTC(),
		}},
	}

	r := chi.NewRouter()

	r.Use(middleware.Logger)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tplIndex.Execute(
			w,
			c.Messages,
		)
	})

	r.Get("/messages", func(w http.ResponseWriter, r *http.Request) {
		tplMessagesNew.Execute(
			w,
			nil,
		)
	})

	r.Post("/messages", func(w http.ResponseWriter, r *http.Request) {
		body := r.PostFormValue("body")
		c.AddMessage(body)
	})

	es := hotwire.NewEventStream()
	r.Get("/events", es.ServeHTTP)

	go func() {
		messages := c.OnAddMessage()
		for {
			message, ok := <-messages
			if !ok {
				return
			}
			es.SendEvent(
				hotwire.StreamActionAppend,
				"room-messages",
				tplMessageInner,
				message,
			)
		}
	}()

	http.ListenAndServe(":3000", r)
}
