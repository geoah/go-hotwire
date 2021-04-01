package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/geoah/go-hotwire"
)

const mimeTurboStream = "text/vnd.turbo-stream.html"

//go:embed assets/*.html
var assets embed.FS

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
		tpl, err := template.ParseFS(
			assets,
			"assets/base.html",
			"assets/frame.room.html",
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tpl.Execute(
			w,
			c.Messages,
		)
	})

	r.Get("/messages", func(w http.ResponseWriter, r *http.Request) {
		tpl, err := template.ParseFS(
			assets,
			"assets/base.html",
			"assets/frame.messages-new.html",
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tpl.Execute(
			w,
			nil,
		)
	})

	r.Post("/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", mimeTurboStream)
		body := r.PostFormValue("body")
		c.AddMessage(body)
	})

	tpl, err := template.ParseFS(
		assets,
		"assets/inner.message.html",
	)
	if err != nil {
		log.Fatal(err)
	}

	es := hotwire.NewEventStream()
	r.Get("/events", es.ServeHTTP)

	go func() {
		ms := c.OnAddMessage()
		for {
			m, ok := <-ms
			if !ok {
				return
			}
			es.SendEvent(hotwire.StreamActionAppend, "room-messages", tpl, m)
		}
	}()

	http.ListenAndServe(":3000", r)
}
