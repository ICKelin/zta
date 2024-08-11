package main

import (
	"github.com/ICKelin/zta/common"
	"github.com/astaxie/beego/logs"
	"net"
	"time"
)

type Gateway struct {
	ListenAddr string
	sessionMgr *SessionManager
}

func NewGateway(listenAddr string, sessionMgr *SessionManager) *Gateway {
	gw := &Gateway{
		ListenAddr: listenAddr,
		sessionMgr: sessionMgr,
	}
	go gw.checkOnlineInterval()
	return gw
}

func (gw *Gateway) ListenAndServe() error {
	listener, err := net.Listen("tcp", gw.ListenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go gw.handleConn(conn)
	}
}

func (gw *Gateway) handleConn(conn net.Conn) {
	handshakeReq := &common.HandshakeReq{}
	err := handshakeReq.Decode(conn)
	if err != nil {
		logs.Error("decode handshake fail: %v", err)
		return
	}

	logs.Debug("handshake from %s", handshakeReq.ClientID)

	// 创建session
	_, err = gw.sessionMgr.CreateSession(handshakeReq.ClientID, conn)
	if err != nil {
		logs.Error("create session fail: %v", err)
		return
	}
}

func (gw *Gateway) checkOnlineInterval() {
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()
	for range tick.C {
		gw.sessionMgr.Range(func(k string, v *Session) bool {
			if v.Connection.IsClosed() {
				logs.Info("session %s is offline", v.ClientID)
				return false
			}

			logs.Debug("session %s is online", v.ClientID)
			return true
		})
	}
}
