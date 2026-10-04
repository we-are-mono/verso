// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"sync"
	"time"
)

// pendingUpload is a verified, private upload waiting on disk at path, bound to
// the session (sid) that chose it. Browser forms carry only its opaque token,
// never a server path. data is what the confirmation page shows about it.
type pendingUpload[T any] struct {
	sid     string
	path    string
	created time.Time
	data    T
}

// pendingUploads holds a session's verified uploads by token. An entry lives
// restoreLifetime; a session keeps at most one, so a fresh upload removes the
// file behind the session's previous one. The zero value is ready to use.
type pendingUploads[T any] struct {
	mu    sync.Mutex
	items map[string]pendingUpload[T]
}

func (p *pendingUploads[T]) put(token string, upload pendingUpload[T]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for key, old := range p.items {
		if old.sid == upload.sid || now.Sub(old.created) > restoreLifetime {
			_ = os.Remove(old.path)
			delete(p.items, key)
		}
	}
	if p.items == nil {
		p.items = make(map[string]pendingUpload[T])
	}
	p.items[token] = upload
}

// get returns the token's upload when it belongs to sid and has not expired. A
// stale or foreign lookup removes the entry and its file.
func (p *pendingUploads[T]) get(token, sid string) (pendingUpload[T], bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	upload, ok := p.items[token]
	if !ok || upload.sid != sid || time.Since(upload.created) > restoreLifetime {
		if ok {
			_ = os.Remove(upload.path)
			delete(p.items, token)
		}
		return pendingUpload[T]{}, false
	}
	return upload, true
}

// consume claims the token — removing it under the lock and reporting whether it
// was present — so two racing applies of one verified upload cannot both run.
func (p *pendingUploads[T]) consume(token string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.items[token]
	delete(p.items, token)
	return ok
}

// clear removes every pending upload and its file.
func (p *pendingUploads[T]) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for token, upload := range p.items {
		_ = os.Remove(upload.path)
		delete(p.items, token)
	}
}
