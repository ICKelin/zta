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

type udpSession struct {
	RemoteAddr string
	LocalAddr  string
	tunnelConn net.Conn
	activeAt   time.Time
}

type udpSessionManager struct {
	sessionsMu sync.Mutex
	sessions   map[string]*udpSession
}

//
//func NewUDPSessionManager() *udpSessionManager {
//	return &udpSessionManager{sessions: make(map[string]*udpSession)}
//}
//
//func (mgr *udpSessionManager)GetOrCreate(key string, tunnelConn net.Conn) (*udpSession,error) {
//	 mgr.sessionsMu.Lock()
//	 defer mgr.sessionsMu.Unlock()
//	 sess := mgr.sessions[key]
//	 if sess == nil {
//		 // create
//		 sess := &session
//	 }
//}

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

	udpSessionMu    sync.Mutex
	udpSessionTable map[string]*udpSession
}

func NewListener(listenerConfig *ListenerConfig,
	sessionMgr *SessionManager) *Listener {
	return &Listener{
		listenerConfig:  listenerConfig,
		close:           make(chan struct{}),
		sessionMgr:      sessionMgr,
		udpSessionTable: make(map[string]*udpSession),
	}
}

func (l *Listener) ListenAndServe() error {
	switch l.listenerConfig.PublicProtocol {
	case "http", "https":
		return l.listenAndServeHTTP()
	case "tcp":
		return l.listenAndServeTCP()
	case "udp":
		return l.listenAndServeUDP()
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

func (l *Listener) listenAndServeUDP() error {
	listenAddr := fmt.Sprintf("%s:%d", l.listenerConfig.PublicIP, l.listenerConfig.PublicPort)
	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	go func() {
		tick := time.NewTicker(time.Second * 10)
		defer tick.Stop()

		for range tick.C {
			l.udpSessionMu.Lock()

			l.udpSessionMu.Unlock()
		}
	}()

	buffer := make([]byte, 1024*64)
	for {
		nr, raddr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			break
		}

		l.udpSessionMu.Lock()
		udpSess := l.udpSessionTable[raddr.String()]
		if udpSess == nil {
			l.udpSessionMu.Unlock()
			tunnelConn, err := l.sessionMgr.GetSessionByClientID(l.listenerConfig.ClientID)
			if err != nil {
				logs.Warn("get session for client %s fail", l.listenerConfig.ClientID)
				continue
			}

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
				continue
			}

			tunnelConn.SetWriteDeadline(time.Now().Add(writeTimeout))
			_, err = tunnelConn.Write(ppBody)
			tunnelConn.SetWriteDeadline(time.Time{})
			if err != nil {
				logs.Warn("write listenerConfig body fail: %v", err)
				l.udpSessionMu.Unlock()
				continue
			}

			udpSess = &udpSession{
				RemoteAddr: raddr.String(),
				LocalAddr:  listenAddr,
				tunnelConn: tunnelConn,
			}

			l.udpSessionMu.Lock()
			l.udpSessionTable[raddr.String()] = udpSess
			l.udpSessionMu.Unlock()
			go l.udpReadFromClient(tunnelConn, raddr, listener)
		}

		packet := common.UDPPacket(buffer[:nr])
		body, err := packet.Encode()
		if err != nil {
			logs.Warn("encode udp packet fail: %v", err)
			continue
		}
		logs.Debug("write udp %d bytes to tunnel client", len(body))
		_, err = udpSess.tunnelConn.Write(body)
		if err != nil {
			// TODO: 清理session
			logs.Warn("write body fail: %v", err)
			continue
		}
	}
	return nil

}

func (l *Listener) udpReadFromClient(tunnelConn net.Conn, raddr *net.UDPAddr, conn *net.UDPConn) {
	buffer := common.UDPPacket(make([]byte, 1024*64))
	for {
		nr, err := buffer.Decode(tunnelConn)
		if err != nil {
			logs.Warn("decode udp from tunnel conn fail: %v", err)
			break
		}

		_, err = conn.WriteToUDP(buffer[:nr], raddr)
		if err != nil {
			logs.Warn("write udp to %v fail: %v", raddr.String(), err)
			break
		}
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
