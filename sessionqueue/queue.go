package sessionqueue

import (
	"log"
	"sync"

	"custom-agent/gateway"
	"custom-agent/session"
)

const jobBufferSize = 32

type job struct {
	msg     gateway.IncomingMessage
	replyCh chan string
}

type worker struct {
	jobs chan job
}

// Queue processes messages one at a time per session. If a message arrives
// while another is still processing, it is queued and processed in order.
type Queue struct {
	handler  func(gateway.IncomingMessage) string
	sessions sync.Map // sessionKey -> *worker
	createMu sync.Mutex
}

// New creates a queue that wraps the given handler.
func New(handler func(gateway.IncomingMessage) string) *Queue {
	return &Queue{handler: handler}
}

// Process submits a message for the session. It blocks until the message is
// processed and the reply is ready. Messages for the same session are
// processed in FIFO order.
func (q *Queue) Process(msg gateway.IncomingMessage) string {
	key := session.SessionKey(msg.Platform, msg.UserID)
	if key == "" || key == "_" {
		return q.handler(msg)
	}

	w := q.getOrCreateWorker(key)
	replyCh := make(chan string, 1)
	w.jobs <- job{msg: msg, replyCh: replyCh}
	return <-replyCh
}

func (q *Queue) getOrCreateWorker(key string) *worker {
	if v, ok := q.sessions.Load(key); ok {
		return v.(*worker)
	}
	q.createMu.Lock()
	defer q.createMu.Unlock()
	if v, ok := q.sessions.Load(key); ok {
		return v.(*worker)
	}
	w := &worker{
		jobs: make(chan job, jobBufferSize),
	}
	q.sessions.Store(key, w)
	go w.run(q.handler)
	return w
}

func (w *worker) run(handler func(gateway.IncomingMessage) string) {
	for j := range w.jobs {
		var result string
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[sessionqueue] handler panic: %v", r)
					result = "Sorry, something went wrong. Please try again."
				}
			}()
			result = handler(j.msg)
		}()
		j.replyCh <- result
	}
}
