package main

import (
	"flag"
	"github.com/ICKelin/zta/gateway/http_router"
)

func main() {
	var confFile string
	flag.StringVar(&confFile, "c", "", "config file")
	flag.Parse()

	conf, err := ParseConfig(confFile)
	if err != nil {
		panic(err)
	}

	listenerConfigs, err := ParseListenerConfig(conf.ListenerFile)
	if err != nil {
		panic(err)
	}

	sessionMgr := NewSessionManager()

	clientIDs := make([]string, 0)
	httpRouter := http_router.NewApisixRoute(&http_router.ApisixConfig{
		Api: "http://127.0.0.1:9180",
		Key: "edd1c9f034335f136f87ad84b625c8f1",
	})

	for _, listenerConfig := range listenerConfigs {
		listener := NewListener(listenerConfig, sessionMgr, httpRouter)
		go func() {
			defer listener.Close()
			err := listener.ListenAndServe()
			if err != nil {
				panic(err)
			}
		}()
		clientIDs = append(clientIDs, listenerConfig.ClientID)
	}

	gw := NewGateway(conf.GatewayConfig, sessionMgr)
	gw.SetAvailableClientIDs(clientIDs)
	err = gw.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
