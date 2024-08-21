package main

import (
	"github.com/astaxie/beego/logs"
	"reflect"
	"time"
)

// WatchListenerFile change and callback f
func WatchListenerFile(gw *Gateway,
	file string,
	listenerMgr *ListenerManager,
	sessionMgr *SessionManager,
	currentListenerConfigs []*ListenerConfig) {
	tick := time.NewTicker(time.Minute * 1)
	defer tick.Stop()
	for range tick.C {
		listenerConfigs, err := ParseListenerConfig(file)
		if err != nil {
			logs.Warn("%v", err)
			continue
		}

		added := getAddedListener(currentListenerConfigs, listenerConfigs)
		deleted := getDeletedListener(currentListenerConfigs, listenerConfigs)

		logs.Info("will add %d delete %d", len(added), len(deleted))
		for _, conf := range added {
			logs.Info("update/add %+v", conf)
			l := NewListener(conf, sessionMgr)
			go l.ListenAndServe()
			listenerMgr.AddListener(conf.ID, l)
		}

		for _, conf := range deleted {
			logs.Info("delete %+v", conf)
			listenerMgr.CloseListener(conf.ID)
		}
		currentListenerConfigs = listenerConfigs

		// update clientIDS
		clientIDs := make([]string, 0)
		for _, l := range listenerConfigs {
			clientIDs = append(clientIDs, l.ClientID)
		}
		gw.SetAvailableClientIDs(clientIDs)
	}
}

func getAddedListener(cur, newest []*ListenerConfig) []*ListenerConfig {
	added := make([]*ListenerConfig, 0)
	for i, newConf := range newest {
		update := true
		for _, oldConf := range cur {
			// new config already exist
			if newConf.ID == oldConf.ID {
				if reflect.DeepEqual(newConf, oldConf) {
					// configuration not changed
					update = false
				}
			}
		}
		if update {
			added = append(added, newest[i])
		}
	}
	return added
}

func getDeletedListener(cur, newest []*ListenerConfig) []*ListenerConfig {
	deleted := make([]*ListenerConfig, 0)
	for i, oldConf := range cur {
		willDelete := true
		for _, newConf := range newest {
			// old config not exist in new config
			if oldConf.ID == newConf.ID {
				willDelete = false
				break
			}
		}
		if willDelete {
			deleted = append(deleted, cur[i])
		}
	}
	return deleted
}
