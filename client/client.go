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
			logs.Error("connect to to local fail: %v", err)
			return
		}
		defer localConn.Close()

		// 双向数据拷贝
		go func() {
			defer localConn.Close()
			defer stream.Close()
			io.Copy(localConn, stream)
		}()
		io.Copy(stream, localConn)

	case "udp":
		localConn, err = net.Dial("udp", fmt.Sprintf("%s:%d", pp.InternalIP, pp.InternalPort))
		if err != nil {
			logs.Error("connect to to local fail: %v", err)
			return
		}
		defer localConn.Close()

		// read local conn
		go func() {
			defer localConn.Close()
			defer stream.Close()
			buf := make([]byte, 1024*64)
			for {
				nr, err := localConn.Read(buf)
				if err != nil {
					logs.Error("read udp from local fail %v", err)
					break
				}

				logs.Debug("read %d bytes from local connect", nr)
				// udp packet编码
				p := common.UDPPacket(buf[:nr])
				body, err := p.Encode()
				if err != nil {
					logs.Warn("encode udp packet fail: %v", err)
					break
				}

				_, err = stream.Write(body)
				if err != nil {
					logs.Warn("write udp to stream fail: %v", err)
					break
				}
			}
		}()

		// read stream
		p := common.UDPPacket(make([]byte, 1024*64))
		for {
			nr, err := p.Decode(stream)
			if err != nil {
				logs.Warn("decode udp from stream fail: %v", err)
				break
			}

			logs.Debug("read from stream %d bytes", nr)
			_, err = localConn.Write(p[:nr])
			if err != nil {
				logs.Warn("write udp to local conn fail: %v", err)
				break
			}
		}

	default:
		logs.Warn("unsupported protocol %s", pp.InternalProtocol)
	}

}
