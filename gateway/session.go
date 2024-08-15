package main

import (
	"fmt"
	"github.com/xtaci/smux"
	"net"
	"sync"
)

type Session struct {
	ClientID   string
	Connection *smux.Session
}

type SessionManager struct {
	sessionsMu sync.Mutex
	sessions   map[string]*Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (mgr *SessionManager) GetSessionByClientID(clientID string) (net.Conn, error) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()
	sess := mgr.sessions[clientID]
	if sess == nil {
		return nil, fmt.Errorf("client %s not connected", clientID)
	}

	stream, err := sess.Connection.OpenStream()
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (mgr *SessionManager) CreateSession(clientID string, conn net.Conn) (*Session, error) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()

	mux, err := smux.Server(conn, nil)
	if err != nil {
		return nil, err
	}

	old := mgr.sessions[clientID]
	if old != nil {
		return nil, fmt.Errorf("client %s is online", clientID)
	}

	sess := &Session{
		ClientID:   clientID,
		Connection: mux,
	}
	mgr.sessions[clientID] = sess
	return sess, nil
}

func (mgr *SessionManager) Range(f func(k string, v *Session) bool) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()

	for k, v := range mgr.sessions {
		ok := f(k, v)
		if !ok {
			delete(mgr.sessions, k)
		}
	}
}
