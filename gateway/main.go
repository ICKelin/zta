package main

import (
	"encoding/json"
	"flag"
	"github.com/ICKelin/zta/gateway/http_route"
	"github.com/astaxie/beego/logs"
)

func main() {
	var confFile string
	flag.StringVar(&confFile, "c", "", "config file")
	flag.Parse()

	// parse main config
	conf, err := ParseConfig(confFile)
	if err != nil {
		panic(err)
	}

	// parse listener config file
	listenerConfigs, err := ParseListenerConfig(conf.ListenerFile)
	if err != nil {
		panic(err)
	}

	// parse ssl config
	sslConfigs, err := ParseSSLConfig(conf.SSLFile)
	if err != nil {
		panic(err)
	}

	// init global http route, for example apisix
	for routeType, routeConfig := range conf.HttpRoutes {
		err := http_route.InitRoute(routeType, json.RawMessage(routeConfig))
		if err != nil {
			panic(err)
		}
	}

	// create ssl config
	for _, sslConfig := range sslConfigs {
		route := http_route.GetRoute(sslConfig.HTTPRouteType)
		err := route.UpdateSSL(sslConfig.ID, sslConfig.Cert, sslConfig.Key, sslConfig.SNIs)
		if err != nil {
			panic(err)
		}
	}

	clientIDs := make([]string, 0)
	sessionMgr := NewSessionManager()
	// listening ports
	for _, listenerConfig := range listenerConfigs {
		listener := NewListener(listenerConfig, sessionMgr)
		go func() {
			defer listener.Close()
			err := listener.ListenAndServe()
			if err != nil {
				logs.Error("listener %s serve fail: %v", listenerConfig.ID, err)
			}
		}()
		clientIDs = append(clientIDs, listenerConfig.ClientID)
	}

	// init tunnel gateway server
	gw := NewGateway(conf.GatewayConfig, sessionMgr)
	gw.SetAvailableClientIDs(clientIDs)
	err = gw.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

// WatchListenerFile change and callback f
//func WatchListenerFile(file string, newestListeners []*Listener,
//	f func(id string, conf *ListenerConfig)) {
//	tick := time.NewTicker(time.Second * 3)
//	defer tick.Stop()
//	for range tick.C {
//		listeners, err := ParseListenerConfig(file)
//		if err != nil {
//			logs.Warn("%v", err)
//			continue
//		}
//
//		newestListeners = listeners
//	}
//}
