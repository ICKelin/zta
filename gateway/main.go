package main

import (
	"flag"
	"github.com/ICKelin/zta/common"
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

	for _, listenerConfig := range listenerConfigs {
		listener := NewListener(&common.ProxyProtocol{
			ClientID:         listenerConfig.ClientID,
			PublicProtocol:   listenerConfig.PublicProtocol,
			PublicIP:         listenerConfig.PublicIP,
			PublicPort:       listenerConfig.PublicPort,
			InternalProtocol: listenerConfig.InternalProtocol,
			InternalIP:       listenerConfig.InternalIP,
			InternalPort:     listenerConfig.InternalPort,
		}, sessionMgr)
		go func() {
			defer listener.Close()
			err := listener.ListenAndServe()
			if err != nil {
				panic(err)
			}
		}()
	}

	gw := NewGateway(":12359", sessionMgr)
	err = gw.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
