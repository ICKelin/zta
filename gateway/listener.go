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
	remoteAddr string
	localAddr  string
	tunnelConn net.Conn
	activeAt   time.Time
}

type udpSessionManager struct {
	sessionsMu sync.Mutex
	sessions   map[string]*udpSession
}

func newUDPSessionManager() *udpSessionManager {
	return &udpSessionManager{sessions: make(map[string]*udpSession)}
}

func (mgr *udpSessionManager) Get(key string) *udpSession {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()
	sess := mgr.sessions[key]
	if sess != nil {
		sess.activeAt = time.Now()
	}
	return sess
}

func (mgr *udpSessionManager) Set(remoteAddr, localAddr string, tunnelConn net.Conn) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()
	sess := &udpSession{
		remoteAddr: remoteAddr,
		localAddr:  localAddr,
		tunnelConn: tunnelConn,
		activeAt:   time.Now(),
	}
	mgr.sessions[localAddr] = sess
}

func (mgr *udpSessionManager) Del(key string) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()
	delete(mgr.sessions, key)
}

func (mgr *udpSessionManager) Range(f func(k string, value *udpSession) bool) {
	mgr.sessionsMu.Lock()
	defer mgr.sessionsMu.Unlock()
	for k, v := range mgr.sessions {
		expired := f(k, v)
		if expired {
			delete(mgr.sessions, k)
		}
	}
}

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
	listenerConfig    *ListenerConfig
	sessionMgr        *SessionManager
	closeOnce         sync.Once
	close             chan struct{}
	tcpListener       net.Listener
	udpSessionManager *udpSessionManager
}

func NewListener(listenerConfig *ListenerConfig,
	sessionMgr *SessionManager) *Listener {
	return &Listener{
		listenerConfig:    listenerConfig,
		close:             make(chan struct{}),
		sessionMgr:        sessionMgr,
		udpSessionManager: newUDPSessionManager(),
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

		go l.handleTCPConn(conn)
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
			l.udpSessionManager.Range(func(k string, value *udpSession) bool {
				if value.activeAt.Add(time.Second * 30).Before(time.Now()) {
					logs.Debug("session %s is expired, last active %d",
						k, value.activeAt.Unix())
					value.tunnelConn.Close()
					return true
				}
				return false
			})
		}
	}()

	buffer := make([]byte, 1024*64)
	for {
		nr, raddr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			break
		}
		l.handleUDPMsg(listener, raddr, buffer[:nr])
	}
	return nil

}

func (l *Listener) handleTCPConn(conn net.Conn) {
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

func (l *Listener) handleUDPMsg(listener *net.UDPConn, raddr *net.UDPAddr, buffer []byte) {
	udpSess := l.udpSessionManager.Get(raddr.String())
	if udpSess == nil {
		// for the first packet
		// 1、encode proxy protocol and send to zta client via tunnel connection
		// 2、create udp session like iptables connection tracking to record udp info
		// 3、bootstrap a goroutine to handle msg from client via tunnel connection
		tunnelConn, err := l.sessionMgr.GetSessionByClientID(l.listenerConfig.ClientID)
		if err != nil {
			logs.Warn("get session for client %s fail", l.listenerConfig.ClientID)
			return
		}

		// 1、encode proxy protocol and send to zta client via tunnel connection
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

		// 2、create udp session like iptables connection tracking to record udp info
		udpSess = &udpSession{
			remoteAddr: raddr.String(),
			localAddr:  listener.LocalAddr().String(),
			tunnelConn: tunnelConn,
		}

		l.udpSessionManager.Set(raddr.String(), listener.LocalAddr().String(), tunnelConn)

		// 3、bootstrap a goroutine to handle msg from client via tunnel connection
		go l.udpReadFromClient(tunnelConn, raddr, listener)
	}

	// Copy buffer to client via tunnel connection
	// for diagram packet, its different with tcp
	// since tunnel connection is stream orient
	// we need to encode a private header to mark different diagram packet
	// for example:
	//  topology: outer udp client ---udp--->[server ---tunnel connect---> client ]---udp---> udp server
	// 	1、outer send 1000 bytes via udp to server
	// 	2、outer send another 1000 bytes via udp to server
	//	3、for server, it reads two msg
	//	4、server sends these two msg to client vial tunnel client
	//	5、since tunnel connection is stream, the client may read 1000+1000 bytes
	//	data at the same time, and sends 2000 bytes to the inner udp server, this may cause exception,
	//	since the outer wants to send two msg, each msg is 1000 bytes, not one msg with 2000 bytes
	packet := common.UDPPacket(buffer)
	body, err := packet.Encode()
	if err != nil {
		l.udpSessionManager.Del(raddr.String())
		logs.Warn("encode udp packet fail: %v", err)
	}
	logs.Debug("write udp %d bytes to tunnel client", len(body))
	_, err = udpSess.tunnelConn.Write(body)
	if err != nil {
		l.udpSessionManager.Del(raddr.String())
		logs.Warn("write body fail: %v", err)
		return
	}
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

func (l *Listener) Close() {
	l.closeOnce.Do(func() {
		close(l.close)
		if l.tcpListener != nil {
			l.tcpListener.Close()
		}
	})
}
