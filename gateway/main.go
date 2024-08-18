package main

import (
	"encoding/json"
	"flag"
	"github.com/ICKelin/zta/gateway/http_route"
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

	// init global http route, for example apisix
	for routeType, routeConfig := range conf.HttpRoutes {
		err := http_route.InitRoute(routeType, json.RawMessage(routeConfig))
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
				panic(err)
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
