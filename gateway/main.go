package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"github.com/ICKelin/zta/gateway/authenticate"
	"github.com/ICKelin/zta/gateway/http_route"
	"github.com/astaxie/beego/logs"
	"gopkg.in/square/go-jose.v1"
)

func main() {
	var confFile string
	flag.StringVar(&confFile, "c", "", "config file")
	flag.Parse()

	go runOIDCService()

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
	listenerMgr := NewListenerManager()
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
		listenerMgr.AddListener(listenerConfig.ID, listener)
		clientIDs = append(clientIDs, listenerConfig.ClientID)
	}
	// init tunnel gateway server
	gw := NewGateway(conf.GatewayConfig, sessionMgr)
	gw.SetAvailableClientIDs(clientIDs)

	if conf.AutoReload {
		// watch listener file for add/delete listeners interval
		go WatchListenerFile(gw, conf.ListenerFile, listenerMgr, sessionMgr, listenerConfigs)
	}
	err = gw.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func runOIDCService() {
	// Load signing key.
	block, _ := pem.Decode(privateKeyBytes)
	if block == nil {
		panic("decode private key fail")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}

	// Configure jwtSigner and public keys.
	privateKey := &jose.JsonWebKey{
		Key:       key,
		Algorithm: "RS256",
		Use:       "sig",
		KeyID:     "1", // KeyID should use the key thumbprint.
	}

	jwtSigner, err := jose.NewSigner(jose.RS256, privateKey)
	if err != nil {
		panic(err)
	}
	publicKeys := &jose.JsonWebKeySet{
		Keys: []jose.JsonWebKey{
			jose.JsonWebKey{Key: &key.PublicKey,
				Algorithm: "RS256",
				Use:       "sig",
				KeyID:     "1",
			},
		},
	}

	oidc := authenticate.NewOIDC(&authenticate.OIDCConfig{
		Issuer:     "http://oidc.zta.beyondnetwork.net:14001",
		ListenAddr: ":14001",
	}, jwtSigner, publicKeys)

	oidc.AddClient("client_id", "client_secret", "http://app2.zta.beyondnetwork.net:9080/.apisix/redirect")
	oidc.AddUser("client_id", &authenticate.UserInfo{
		Username: "username",
		Password: "password",
		Email:    "yingjiu.hulu@gmail.com",
	})
	oidc.Serve()
}

var (
	privateKeyBytes = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA4f5wg5l2hKsTeNem/V41fGnJm6gOdrj8ym3rFkEU/wT8RDtn
SgFEZOQpHEgQ7JL38xUfU0Y3g6aYw9QT0hJ7mCpz9Er5qLaMXJwZxzHzAahlfA0i
cqabvJOMvQtzD6uQv6wPEyZtDTWiQi9AXwBpHssPnpYGIn20ZZuNlX2BrClciHhC
PUIIZOQn/MmqTD31jSyjoQoV7MhhMTATKJx2XrHhR+1DcKJzQBSTAGnpYVaqpsAR
ap+nwRipr3nUTuxyGohBTSmjJ2usSeQXHI3bODIRe1AuTyHceAbewn8b462yEWKA
Rdpd9AjQW5SIVPfdsz5B6GlYQ5LdYKtznTuy7wIDAQABAoIBAQCwia1k7+2oZ2d3
n6agCAbqIE1QXfCmh41ZqJHbOY3oRQG3X1wpcGH4Gk+O+zDVTV2JszdcOt7E5dAy
MaomETAhRxB7hlIOnEN7WKm+dGNrKRvV0wDU5ReFMRHg31/Lnu8c+5BvGjZX+ky9
POIhFFYJqwCRlopGSUIxmVj5rSgtzk3iWOQXr+ah1bjEXvlxDOWkHN6YfpV5ThdE
KdBIPGEVqa63r9n2h+qazKrtiRqJqGnOrHzOECYbRFYhexsNFz7YT02xdfSHn7gM
IvabDDP/Qp0PjE1jdouiMaFHYnLBbgvlnZW9yuVf/rpXTUq/njxIXMmvmEyyvSDn
FcFikB8pAoGBAPF77hK4m3/rdGT7X8a/gwvZ2R121aBcdPwEaUhvj/36dx596zvY
mEOjrWfZhF083/nYWE2kVquj2wjs+otCLfifEEgXcVPTnEOPO9Zg3uNSL0nNQghj
FuD3iGLTUBCtM66oTe0jLSslHe8gLGEQqyMzHOzYxNqibxcOZIe8Qt0NAoGBAO+U
I5+XWjWEgDmvyC3TrOSf/KCGjtu0TSv30ipv27bDLMrpvPmD/5lpptTFwcxvVhCs
2b+chCjlghFSWFbBULBrfci2FtliClOVMYrlNBdUSJhf3aYSG2Doe6Bgt1n2CpNn
/iu37Y3NfemZBJA7hNl4dYe+f+uzM87cdQ214+jrAoGAXA0XxX8ll2+ToOLJsaNT
OvNB9h9Uc5qK5X5w+7G7O998BN2PC/MWp8H+2fVqpXgNENpNXttkRm1hk1dych86
EunfdPuqsX+as44oCyJGFHVBnWpm33eWQw9YqANRI+pCJzP08I5WK3osnPiwshd+
hR54yjgfYhBFNI7B95PmEQkCgYBzFSz7h1+s34Ycr8SvxsOBWxymG5zaCsUbPsL0
4aCgLScCHb9J+E86aVbbVFdglYa5Id7DPTL61ixhl7WZjujspeXZGSbmq0Kcnckb
mDgqkLECiOJW2NHP/j0McAkDLL4tysF8TLDO8gvuvzNC+WQ6drO2ThrypLVZQ+ry
eBIPmwKBgEZxhqa0gVvHQG/7Od69KWj4eJP28kq13RhKay8JOoN0vPmspXJo1HY3
CKuHRG+AP579dncdUnOMvfXOtkdM4vk0+hWASBQzM9xzVcztCa+koAugjVaLS9A+
9uQoqEeVNTckxx0S2bYevRy7hGQmUJTyQm3j1zEUR5jpdbL83Fbq
-----END RSA PRIVATE KEY-----`)
)
