package main

import (
	"github.com/ICKelin/zta/common"
	"github.com/astaxie/beego/logs"
	"net"
	"time"
)

type Gateway struct {
	conf       *GatewayConfig
	clientIDs  map[string]struct{}
	sessionMgr *SessionManager
}

func NewGateway(conf *GatewayConfig, sessionMgr *SessionManager) *Gateway {
	gw := &Gateway{
		conf:       conf,
		sessionMgr: sessionMgr,
	}
	go gw.checkOnlineInterval()
	return gw
}

func (gw *Gateway) SetAvailableClientIDs(clientIDs []string) {
	clientIDsMap := make(map[string]struct{})
	for _, clientID := range clientIDs {
		clientIDsMap[clientID] = struct{}{}
	}
	gw.clientIDs = clientIDsMap
}

func (gw *Gateway) ListenAndServe() error {
	listener, err := net.Listen("tcp", gw.conf.ListenAddr)
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

	if _, ok := gw.clientIDs[handshakeReq.ClientID]; !ok {
		logs.Warn("client %s is not configured", handshakeReq.ClientID)
		return
	}

	logs.Debug("handshake from %s", handshakeReq.ClientID)

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
