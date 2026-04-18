package extensions

import "sync"

type stateLock struct {
	mu    sync.Mutex
	users int
}

type stateStore struct {
	mu      sync.RWMutex
	items   map[string]ExtensionInfo
	locks   map[string]*stateLock
	idle    map[string]struct{}
	events  []ExtensionEvent
	loading map[string]bool
}

func newStateStore() *stateStore {
	return &stateStore{
		items:   map[string]ExtensionInfo{},
		locks:   map[string]*stateLock{},
		idle:    map[string]struct{}{},
		events:  nil,
		loading: map[string]bool{},
	}
}

func (s *stateStore) withLock(id string, fn func() error) error {
	lock := s.lockFor(id)
	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		s.releaseLock(id, lock)
	}()
	return fn()
}

func (s *stateStore) lockFor(id string) *stateLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, ok := s.locks[id]
	if !ok {
		lock = &stateLock{}
		s.locks[id] = lock
	}
	lock.users++
	delete(s.idle, id)
	return lock
}

func (s *stateStore) releaseLock(id string, lock *stateLock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.locks[id]
	if !ok || current != lock {
		return
	}
	if current.users > 0 {
		current.users--
	}
	if current.users == 0 {
		s.idle[id] = struct{}{}
	}
	s.gcIdleLocksLocked()
}

func (s *stateStore) gcIdleLocksLocked() {
	for id := range s.idle {
		if _, itemExists := s.items[id]; itemExists {
			continue
		}
		if s.loading[id] {
			continue
		}
		lock, ok := s.locks[id]
		if !ok || lock.users != 0 {
			continue
		}
		delete(s.locks, id)
		delete(s.idle, id)
	}
}

func (s *stateStore) beginLoad(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loading[id] {
		return wrapError(ErrCodeBusy, "extension is busy", nil)
	}
	if _, ok := s.items[id]; ok {
		return wrapError(ErrCodeAlreadyLoaded, "extension already loaded", nil)
	}
	s.loading[id] = true
	return nil
}

func (s *stateStore) finishLoad(id string, info ExtensionInfo, events ...ExtensionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loading, id)
	delete(s.idle, id)
	s.items[id] = cloneExtensionInfo(info)
	s.events = append(s.events, cloneEvents(events)...)
}

func (s *stateStore) cancelLoad(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loading, id)
	s.gcIdleLocksLocked()
}

func (s *stateStore) get(id string) (ExtensionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.items[id]
	return cloneExtensionInfo(info), ok
}

func (s *stateStore) set(info ExtensionInfo, events ...ExtensionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.idle, info.ID)
	s.items[info.ID] = cloneExtensionInfo(info)
	s.events = append(s.events, cloneEvents(events)...)
}

func (s *stateStore) delete(id string, events ...ExtensionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	delete(s.loading, id)
	s.events = append(s.events, cloneEvents(events)...)
	s.gcIdleLocksLocked()
}

func (s *stateStore) list() []ExtensionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]ExtensionInfo, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, cloneExtensionInfo(item))
	}
	return items
}

func cloneExtensionInfo(info ExtensionInfo) ExtensionInfo {
	return info
}

func cloneEvents(events []ExtensionEvent) []ExtensionEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]ExtensionEvent, len(events))
	copy(out, events)
	return out
}
