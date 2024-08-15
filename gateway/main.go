package main

import (
	"flag"
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

	gw := NewGateway(conf.GatewayConfig, sessionMgr)
	gw.SetAvailableClientIDs(clientIDs)
	err = gw.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
