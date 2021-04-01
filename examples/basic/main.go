package main

import (
	"embed"
	"fmt"
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
	tplMessagesEdit = template.Must(template.ParseFS(
		assets,
		"assets/base.html",
		"assets/frame.messages-edit.html",
	))
	tplMessageInner = template.Must(template.ParseFS(
		assets,
		"assets/inner.message.html",
	))
)

type (
	Room struct {
		sync.RWMutex
		Messages []*Message
	}
	Message struct {
		ID      string
		Body    string
		Created time.Time
	}
)

func (r *Room) AddMessage(body string) *Message {
	r.Lock()
	defer r.Unlock()
	m := &Message{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Body:    body,
		Created: time.Now().UTC().Truncate(time.Second),
	}
	r.Messages = append(r.Messages, m)
	return m
}

func (r *Room) UpdateMessage(id, body string) (*Message, error) {
	message, err := r.GetMessage(id)
	if err != nil {
		return nil, err
	}
	r.Lock()
	defer r.Unlock()
	message.Body = body
	return message, nil
}

func (r *Room) GetMessage(id string) (*Message, error) {
	r.Lock()
	defer r.Unlock()
	for _, m := range r.Messages {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, fmt.Errorf("message not found")
}

func (r *Room) RemoveMessage(id string) {
	r.Lock()
	defer r.Unlock()
	j := -1
	for i, m := range r.Messages {
		if m.ID == id {
			j = i
			break
		}
	}
	if j < 0 {
		return
	}
	r.Messages = append(r.Messages[:j], r.Messages[j+1:]...)
}

func (r *Room) ListMessages() []*Message {
	r.RLock()
	defer r.RUnlock()
	return r.Messages
}

func main() {
	c := Room{
		RWMutex: sync.RWMutex{},
		Messages: []*Message{{
			ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
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

	es := hotwire.NewEventStream()
	r.Get("/events", es.ServeHTTP)

	r.Get("/messages/new", func(w http.ResponseWriter, r *http.Request) {
		tplMessagesNew.Execute(
			w,
			nil,
		)
	})

	r.Post("/messages/new", func(w http.ResponseWriter, r *http.Request) {
		body := r.PostFormValue("body")
		message := c.AddMessage(body)
		// send stream event to add the message
		es.SendEvent(
			hotwire.StreamActionAppend,
			"room-messages",
			tplMessageInner,
			message,
		)
	})

	r.Get("/messages/edit", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		message, err := c.GetMessage(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		tplMessagesEdit.Execute(
			w,
			message,
		)
	})

	r.Post("/messages/edit", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		body := r.PostFormValue("body")
		message, err := c.UpdateMessage(id, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		// send stream event to update the message
		es.SendEvent(
			hotwire.StreamActionReplace,
			"message-"+id,
			tplMessageInner,
			message,
		)
		// redirect to /messages/new
		http.Redirect(w, r, "/messages/new", http.StatusFound)
	})

	r.Get("/messages/delete", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		c.RemoveMessage(id)
		// send stream event to remove the message
		es.SendEvent(
			hotwire.StreamActionRemove,
			"message-"+id,
			tplMessageInner,
			nil,
		)
	})

	http.ListenAndServe(":3000", r)
}
