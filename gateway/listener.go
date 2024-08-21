package main

import (
	"fmt"
	"github.com/ICKelin/zta/common"
	"github.com/ICKelin/zta/gateway/http_route"
	"github.com/astaxie/beego/logs"
	"io"
	"net"
	"sync"
	"time"
)

var (
	writeTimeout = time.Second * 3
)

type ListenerManager struct {
	listenersMu sync.Mutex
	listeners   map[string]*Listener
}

func NewListenerManager() *ListenerManager {
	return &ListenerManager{listeners: make(map[string]*Listener)}
}

func (mgr *ListenerManager) AddListener(id string, l *Listener) {
	mgr.listenersMu.Lock()
	defer mgr.listenersMu.Unlock()
	mgr.listeners[id] = l
}

func (mgr *ListenerManager) CloseListener(id string) {
	mgr.listenersMu.Lock()
	defer mgr.listenersMu.Unlock()
	l := mgr.listeners[id]
	if l != nil {
		l.Close()
		delete(mgr.listeners, id)
	}
}

type Listener struct {
	listenerConfig *ListenerConfig
	sessionMgr     *SessionManager
	closeOnce      sync.Once
	close          chan struct{}
	tcpListener    net.Listener
}

func NewListener(listenerConfig *ListenerConfig,
	sessionMgr *SessionManager) *Listener {
	return &Listener{
		listenerConfig: listenerConfig,
		close:          make(chan struct{}),
		sessionMgr:     sessionMgr,
	}
}

func (l *Listener) ListenAndServe() error {
	switch l.listenerConfig.PublicProtocol {
	case "http", "https":
		return l.listenAndServeHTTP()
	case "tcp":
		return l.listenAndServeTCP()
	default:
		return fmt.Errorf("TODO://")
	}
}

func (l *Listener) listenAndServeHTTP() error {
	route := http_route.GetRoute(l.listenerConfig.HTTPRouteType)
	if route == nil {
		return fmt.Errorf("route %s is not initialize",
			l.listenerConfig.HTTPRouteType)
	}

	// update http_route rule
	err := route.UpdateRoute(l.listenerConfig.HTTPParam)
	if err != nil {
		return err
	}

	// listening and serve tcp for http(s)
	return l.listenAndServeTCP()
}

func (l *Listener) listenAndServeTCP() error {
	listenAddr := fmt.Sprintf("%s:%d", l.listenerConfig.PublicIP, l.listenerConfig.PublicPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	l.tcpListener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(conn net.Conn) {
	defer conn.Close()

	// get session for clientID
	tunnelConn, err := l.sessionMgr.GetSessionByClientID(l.listenerConfig.ClientID)
	if err != nil {
		logs.Warn("get session for client %s fail", l.listenerConfig.ClientID)
		return
	}
	defer tunnelConn.Close()

	// encode and send pp to client
	pp := &common.ProxyProtocol{
		ClientID:         l.listenerConfig.ClientID,
		PublicProtocol:   l.listenerConfig.PublicProtocol,
		PublicIP:         l.listenerConfig.PublicIP,
		PublicPort:       l.listenerConfig.PublicPort,
		InternalProtocol: l.listenerConfig.InternalProtocol,
		InternalIP:       l.listenerConfig.InternalIP,
		InternalPort:     l.listenerConfig.InternalPort,
	}
	ppBody, err := pp.Encode()
	if err != nil {
		logs.Warn("encode listenerConfig fail: %v ", err)
		return
	}

	tunnelConn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, err = tunnelConn.Write(ppBody)
	tunnelConn.SetWriteDeadline(time.Time{})
	if err != nil {
		logs.Warn("write listenerConfig body fail: %v", err)
		return
	}

	// copy from and copy to .
	go func() {
		defer tunnelConn.Close()
		defer conn.Close()
		io.Copy(tunnelConn, conn)
	}()
	io.Copy(conn, tunnelConn)
}

func (l *Listener) Close() {
	l.closeOnce.Do(func() {
		close(l.close)
		if l.tcpListener != nil {
			l.tcpListener.Close()
		}
	})
}
