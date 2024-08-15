package main

import (
	"fmt"
	"github.com/ICKelin/zta/common"
	"github.com/ICKelin/zta/gateway/http_router"
	"github.com/astaxie/beego/logs"
	"io"
	"net"
	"sync"
	"time"
)

var (
	writeTimeout = time.Second * 3
)

type Listener struct {
	listenerConfig *ListenerConfig
	sessionMgr     *SessionManager
	closeOnce      sync.Once
	close          chan struct{}
	tcpListener    net.Listener
	httpRouter     http_router.HTTPRouter
}

func NewListener(listenerConfig *ListenerConfig,
	sessionMgr *SessionManager,
	httpRouter http_router.HTTPRouter) *Listener {
	return &Listener{
		listenerConfig: listenerConfig,
		close:          make(chan struct{}),
		sessionMgr:     sessionMgr,
		httpRouter:     httpRouter,
	}
}

func (l *Listener) ListenAndServe() error {
	switch l.listenerConfig.PublicProtocol {
	case "http":
		return l.listenAndServeHTTP()
	case "tcp":
		return l.listenAndServeTCP()
	default:
		return fmt.Errorf("TODO://")
	}
}

func (l *Listener) listenAndServeHTTP() error {
	// 更新http_router配置
	err := l.httpRouter.UpdateRoute(l.listenerConfig.HTTPParam)
	if err != nil {
		return err
	}

	// 监听tcp
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

	// 查询session
	tunnelConn, err := l.sessionMgr.GetSessionByClientID(l.listenerConfig.ClientID)
	if err != nil {
		logs.Warn("get session for client %s fail", l.listenerConfig.ClientID)
		return
	}
	defer tunnelConn.Close()

	// 封装proxyprotocol
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

	// 双向数据拷贝
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
