package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	eventQueueLimit  = 1000
	eventWorkerCount = 4
	eventMaxAge      = 30 * time.Second
)

type queuedLiveEvent struct {
	event      LiveEvent
	enqueuedAt time.Time
}

func (s *AppService) startEventWorkers() {
	for worker := 0; worker < eventWorkerCount; worker++ {
		s.eventWorkers.Add(1)
		go func() {
			defer s.eventWorkers.Done()
			for {
				queued, ok := s.nextQueuedEvent()
				if !ok {
					return
				}
				if time.Since(queued.enqueuedAt) > eventMaxAge {
					s.addDroppedEvents(1)
					continue
				}
				s.processEvent(s.eventCtx, queued.event)
			}
		}()
	}
}

func (s *AppService) enqueueEvent(event LiveEvent) bool {
	s.eventMu.Lock()
	if s.eventStopping {
		s.eventMu.Unlock()
		return false
	}

	key := eventDedupeKey(event)
	if previous, exists := s.eventDedupe[key]; exists && event.Timestamp-previous < 3000 {
		s.eventMu.Unlock()
		return false
	}
	s.eventDedupe[key] = event.Timestamp
	for key, timestamp := range s.eventDedupe {
		if event.Timestamp-timestamp > 30_000 {
			delete(s.eventDedupe, key)
		}
	}

	dropped := 0
	if len(s.eventQueue) >= eventQueueLimit {
		if event.Kind == KindGift {
			removed := false
			for index, queued := range s.eventQueue {
				if queued.event.Kind == KindGift {
					continue
				}
				copy(s.eventQueue[index:], s.eventQueue[index+1:])
				s.eventQueue[len(s.eventQueue)-1] = queuedLiveEvent{}
				s.eventQueue = s.eventQueue[:len(s.eventQueue)-1]
				removed, dropped = true, 1
				break
			}
			if !removed {
				dropped = 1
				s.eventMu.Unlock()
				s.addDroppedEvents(dropped)
				return false
			}
		} else {
			dropped = 1
			s.eventMu.Unlock()
			s.addDroppedEvents(dropped)
			return false
		}
	}

	s.eventQueue = append(s.eventQueue, queuedLiveEvent{event: event, enqueuedAt: time.Now()})
	s.eventCond.Broadcast()
	s.eventMu.Unlock()

	s.mu.Lock()
	if event.Timestamp > s.connection.LastEventAt {
		s.connection.LastEventAt = event.Timestamp
	}
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
	if dropped > 0 {
		s.addDroppedEvents(dropped)
	}
	return true
}

func (s *AppService) nextQueuedEvent() (queuedLiveEvent, bool) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	for len(s.eventQueue) == 0 && !s.eventStopping {
		s.eventCond.Wait()
	}
	if s.eventStopping {
		return queuedLiveEvent{}, false
	}
	queued := s.eventQueue[0]
	copy(s.eventQueue, s.eventQueue[1:])
	s.eventQueue[len(s.eventQueue)-1] = queuedLiveEvent{}
	s.eventQueue = s.eventQueue[:len(s.eventQueue)-1]
	return queued, true
}

func (s *AppService) addDroppedEvents(count int) {
	if count < 1 {
		return
	}
	s.mu.Lock()
	s.connection.Dropped += count
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
}

func (s *AppService) stopEventWorkers() {
	if s.eventCancel != nil {
		s.eventCancel()
	}
	s.eventMu.Lock()
	s.eventStopping = true
	s.eventQueue = nil
	if s.eventCond != nil {
		s.eventCond.Broadcast()
	}
	s.eventMu.Unlock()
	s.eventWorkers.Wait()
}

func eventDedupeKey(event LiveEvent) string {
	user := ""
	if event.User != nil {
		user = event.User.ID
		if user == "" {
			user = event.User.Name
		}
	}
	gift := ""
	if event.Gift != nil {
		gift = event.Gift.Name
	}
	return strings.Join([]string{
		event.Source,
		user,
		string(event.Kind),
		gift,
		event.Text,
		fmt.Sprint(event.Timestamp / 1000),
	}, "|")
}
