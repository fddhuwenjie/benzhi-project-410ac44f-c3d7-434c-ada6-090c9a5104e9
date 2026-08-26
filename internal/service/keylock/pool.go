package keylock

import "sync"

type Pool struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

type Lease struct {
	pool *Pool
	key  string
	lock *sync.Mutex
}

func NewPool() *Pool {
	return &Pool{locks: make(map[string]*sync.Mutex)}
}

func (p *Pool) Reserve(key string) *Lease {
	p.mu.Lock()
	lock := p.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		p.locks[key] = lock
	}
	p.mu.Unlock()
	return &Lease{pool: p, key: key, lock: lock}
}

func (l *Lease) Lock() func() {
	l.lock.Lock()
	return l.release
}

func (l *Lease) TryLock() (func(), bool) {
	if !l.lock.TryLock() {
		return nil, false
	}
	return l.release, true
}

func (l *Lease) release() {
	l.lock.Unlock()
	l.pool.mu.Lock()
	delete(l.pool.locks, l.key)
	l.pool.mu.Unlock()
}
