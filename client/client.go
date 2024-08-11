package main

import (
	"fmt"
	"github.com/ICKelin/zta/common"
	"github.com/astaxie/beego/logs"
	"github.com/xtaci/smux"
	"io"
	"net"
	"time"
)

type Client struct {
	clientID   string
	serverAddr string
}

func NewClient(clientID string, serverAddr string) *Client {
	return &Client{
		clientID,
		serverAddr,
	}
}

func (c *Client) Run() {
	for {
		err := c.run()
		if err != nil && err != io.EOF {
			logs.Error("%v", err)
		}
		logs.Warn("reconnect %s", c.serverAddr)
		time.Sleep(time.Second * 1)
	}
}

func (c *Client) run() error {
	conn, err := net.Dial("tcp", c.serverAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 发送handshake包
	handshakeReq := common.HandshakeReq{ClientID: c.clientID}
	buf, err := handshakeReq.Encode()
	if err != nil {
		return err
	}

	conn.SetWriteDeadline(time.Now().Add(time.Second * 3))
	_, err = conn.Write(buf)
	conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return err
	}

	// 创建mux session
	mux, err := smux.Client(conn, nil)
	if err != nil {
		return err
	}
	defer mux.Close()

	// 等待mux stream
	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			return err
		}

		go c.handleStream(stream)
	}
}

func (c *Client) handleStream(stream net.Conn) {
	defer stream.Close()

	// pp解码
	pp := &common.ProxyProtocol{}
	err := pp.Decode(stream)
	if err != nil {
		logs.Error("decode pp fail: %v", err)
		return
	}
	logs.Debug("pp %+v", pp)

	// 与本地建连接
	var localConn net.Conn
	switch pp.InternalProtocol {
	case "tcp":
		localConn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", pp.InternalIP, pp.InternalPort))
		if err != nil {
			logs.Error("connecto to local fail: %v", err)
			return
		}
		defer localConn.Close()
	default:
		logs.Warn("unsupported protocol %s", pp.InternalProtocol)
	}

	// 双向数据拷贝
	go func() {
		defer localConn.Close()
		defer stream.Close()
		io.Copy(localConn, stream)
	}()
	io.Copy(stream, localConn)
}
