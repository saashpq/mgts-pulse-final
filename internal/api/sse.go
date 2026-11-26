package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEEvent — событие для живого обновления дашборда.
type SSEEvent struct {
	Event string
	Data  string
}

// Broker рассылает события всем открытым дашбордам.
type Broker struct {
	mu     sync.RWMutex
	subs   map[chan SSEEvent]bool
	closed bool
}

func NewBroker() *Broker { return &Broker{subs: map[chan SSEEvent]bool{}} }

func (b *Broker) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch) // сервер останавливается: поток сразу завершится
		return ch
	}
	b.subs[ch] = true
	return ch
}

// Close закрывает все SSE-потоки (вызывается при остановке сервера через srv.RegisterOnShutdown),
// иначе Shutdown ждал бы открытые дашборды до таймаута.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}

func (b *Broker) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[ch] {
		delete(b.subs, ch)
		close(ch)
	}
}

func (b *Broker) Publish(ev SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // медленный клиент не тормозит остальных; он перечитает данные при следующем событии
		}
	}
}

// events — SSE-поток: заголовки и комментарий-приветствие сразу (onopen срабатывает), heartbeat раз в 20 с.
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "internal", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ch := a.Broker.Subscribe()
	defer a.Broker.Unsubscribe(ch)
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, ev.Data)
			fl.Flush()
		}
	}
}
