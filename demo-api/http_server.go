package main

import (
	"encoding/json"
	"github.com/astaxie/beego/logs"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-uuid"
	cron "github.com/robfig/cron/v3"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	listenersFile = "listeners.json"
)

const (
	maxApply = 1000
)

type ApiServer struct {
	addr       string
	todayApply int
	waitListMu sync.Mutex
	waitList   []*ApplyReply
	apply      []*ListenerConfig
}

func NewApiServer(addr string) *ApiServer {
	s := &ApiServer{
		addr:       addr,
		waitListMu: sync.Mutex{},
		waitList:   make([]*ApplyReply, 0),
		apply:      make([]*ListenerConfig, 0),
	}

	// load listeners
	content, err := os.ReadFile(listenersFile)
	if err == nil {
		json.Unmarshal(content, &s.apply)
		s.todayApply = len(s.apply)
	}

	go s.applyInterval()
	return s
}

type ListenerConfig struct {
	ID               string                 `json:"id"`
	ClientID         string                 `json:"client_id"`
	PublicProtocol   string                 `json:"public_protocol"`
	PublicIP         string                 `json:"public_ip"`
	PublicPort       uint16                 `json:"public_port"`
	InternalProtocol string                 `json:"internal_protocol"`
	InternalIP       string                 `json:"internal_ip"`
	InternalPort     uint16                 `json:"internal_port"`
	HTTPRouteType    string                 `json:"http_route_type"`
	HTTPParam        map[string]interface{} `json:"http_param"`
}

func (s *ApiServer) applyInterval() {
	tick := time.NewTicker(time.Minute * 1)
	defer tick.Stop()

	c := cron.New()
	c.AddFunc("0 0 * * *", func() {
		s.waitListMu.Lock()
		defer s.waitListMu.Unlock()
		s.apply = make([]*ListenerConfig, 0)
		s.todayApply = 0
		logs.Debug("reset all applies")
	})
	go c.Run()

	for range tick.C {
		s.waitListMu.Lock()
		logs.Debug("will apply %d request", len(s.waitList))
		if len(s.waitList) <= 0 {
			s.waitListMu.Unlock()
			continue
		}
		s.todayApply += len(s.waitList)

		// 生成配置文件
		listeners := make([]*ListenerConfig, 0)
		for _, apply := range s.waitList {
			listeners = append(listeners, &ListenerConfig{
				ID:               apply.UUID,
				ClientID:         apply.UUID,
				PublicProtocol:   apply.Protocol,
				PublicIP:         "0.0.0.0",
				PublicPort:       apply.PublicPort,
				InternalProtocol: apply.Protocol,
				InternalIP:       apply.IP,
				InternalPort:     apply.Port,
			})
		}

		s.apply = append(s.apply, listeners...)
		s.updateListenerFile()
		s.waitList = make([]*ApplyReply, 0)
		s.waitListMu.Unlock()
	}
}

func (s *ApiServer) updateListenerFile() {
	// 覆盖配置文件
	b, err := json.Marshal(s.apply)
	if err != nil {
		logs.Warn("internal error : %v", err)
		return
	}

	// 覆盖waitList文件
	fp, err := os.Create("listeners.tmp")
	if err != nil {
		logs.Warn("create listener tmp file fail: %v", err)
		return
	}
	defer fp.Close()
	_, err = fp.Write(b)
	if err != nil {
		logs.Warn("create listener tmp file fail: %v", err)
		return
	}

	// move
	_, err = exec.Command("mv", []string{"listeners.tmp", listenersFile}...).Output()
	if err != nil {
		logs.Warn("create listener tmp file fail: %v", err)
	}
}

func (s *ApiServer) ListenAndServe() error {
	srv := gin.Default()
	srv.Use(Cors())

	srv.POST("/apply", s.requestDemo)
	return srv.Run(s.addr)
}

type ApplyForm struct {
	Protocol string `json:"protocol"`
	IP       string `json:"ip"`
	Port     uint16 `json:"port"`
}

type ApplyReply struct {
	Protocol   string `json:"protocol"`
	PublicIP   string `json:"public_ip"`
	PublicPort uint16 `json:"public_port"`
	IP         string `json:"ip"`
	Port       uint16 `json:"port"`
	UUID       string `json:"uuid"`
	CreatedAt  int64  `json:"created_at"`
}

// request for demo
func (s *ApiServer) requestDemo(ctx *gin.Context) {
	f := ApplyForm{}
	err := ctx.BindJSON(&f)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	uuid, err := uuid.GenerateUUID()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}
	uuid = strings.Replace(uuid, "-", "", -1)

	s.waitListMu.Lock()
	defer s.waitListMu.Unlock()
	if s.todayApply >= maxApply {
		ctx.JSON(http.StatusTooManyRequests, nil)
		return
	}

	reply := &ApplyReply{
		Protocol:   f.Protocol,
		PublicIP:   "zta.beyondnetwork.net",
		PublicPort: uint16(s.todayApply + len(s.waitList) + 20000),
		IP:         f.IP,
		Port:       f.Port,
		UUID:       uuid,
		CreatedAt:  time.Now().Unix(),
	}
	s.waitList = append(s.waitList, reply)
	ctx.JSON(http.StatusOK, reply)
}
