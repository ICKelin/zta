package authenticate

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/astaxie/beego/logs"
	"github.com/openshift/osin"
	jose "gopkg.in/square/go-jose.v1"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ Authenticate = &OIDC{}

var (
	jwtSigner  jose.Signer
	publicKeys *jose.JsonWebKeySet
)

type OIDCConfig struct {
	ISSuer     string
	ListenAddr string
}

type OIDC struct {
	conf   *OIDCConfig
	server *osin.Server

	memStorage *MemStorage
	// client_Id -> user list
	usersMu sync.Mutex
	users   map[string][]*UserInfo
}

func NewOIDC(conf *OIDCConfig) *OIDC {
	memStorage := NewMemStorage()
	return &OIDC{
		conf:       conf,
		server:     osin.NewServer(osin.NewServerConfig(), memStorage),
		users:      make(map[string][]*UserInfo),
		memStorage: memStorage,
	}
}

func (o *OIDC) Serve() error {
	http.HandleFunc("/.well-known/openid-configuration", o.handleDiscovery)
	http.HandleFunc("/authorize", o.handleAuthorization)
	http.HandleFunc("/token", o.handleToken)

	return http.ListenAndServe(":14001", nil)
}

func (o *OIDC) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	// For other example see: https://accounts.google.com/.well-known/openid-configuration
	data := map[string]interface{}{
		"issuer":                                o.conf.ISSuer,
		"authorization_endpoint":                o.conf.ISSuer + "/authorize",
		"token_endpoint":                        o.conf.ISSuer + "/token",
		"jwks_uri":                              o.conf.ISSuer + "/publickeys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "email", "profile"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		"claims_supported": []string{
			"aud", "email", "email_verified", "exp",
			"family_name", "given_name", "iat", "iss",
			"locale", "name", "sub",
		},
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logs.Error("failed to marshal data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.Write(raw)
}

func (o *OIDC) handleAuthorization(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	if r.Method == "GET" {
		o.loginPage(w, r)
		return
	}

	resp := o.server.NewResponse()
	defer resp.Close()

	ar := o.server.HandleAuthorizeRequest(resp, r)
	if ar == nil {
		if resp.IsError && resp.InternalError != nil {
			logs.Error("internal error: %v", resp.InternalError)
		}
		osin.OutputJSON(resp, w, r)
		return
	}

	username := r.FormValue("login")
	password := r.FormValue("password")
	// 认证
	user, ok := o.validateUser(ar.Client.GetId(), username, password)
	if !ok {
		resp.SetError(ar.Client.GetId(), "invalid username and passsowd")
		osin.OutputJSON(resp, w, r)
		return
	}

	ar.Authorized = true
	scopes := make(map[string]bool)
	for _, s := range strings.Fields(ar.Scope) {
		scopes[s] = true
	}

	if scopes["openid"] {
		// These values would be tied to the end user authorizing the client.
		now := time.Now()
		idToken := IDToken{
			Issuer:     o.conf.ISSuer,
			UserID:     user.Username,
			ClientID:   ar.Client.GetId(),
			Expiration: now.Add(time.Hour).Unix(),
			IssuedAt:   now.Unix(),
			Nonce:      r.URL.Query().Get("nonce"),
		}

		if scopes["profile"] {
			idToken.Name = user.Username
		}

		if scopes["email"] {
			idToken.Email = user.Email
		}
		ar.UserData = &idToken
	}
	o.server.FinishAuthorizeRequest(resp, r, ar)
}

func (o *OIDC) handleToken(w http.ResponseWriter, r *http.Request) {
	resp := o.server.NewResponse()
	defer resp.Close()

	if ar := o.server.HandleAccessRequest(resp, r); ar != nil {
		ar.Authorized = true
		o.server.FinishAccessRequest(resp, r, ar)

		// If an ID Token was encoded as the UserData, serialize and sign it.
		if idToken, ok := ar.UserData.(*IDToken); ok && idToken != nil {
			encodeIDToken(resp, idToken, jwtSigner)
		}
	}
	if resp.IsError && resp.InternalError != nil {
		fmt.Printf("ERROR: %s\n", resp.InternalError)
	}
	osin.OutputJSON(resp, w, r)
}
func (o *OIDC) loginPage(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	w.Write([]byte("<html><body>"))

	w.Write([]byte(fmt.Sprintf("LOGIN %s (use test/test)<br/>", ar.Client.GetId())))
	w.Write([]byte(fmt.Sprintf("<form action=\"/authorize?%s\" method=\"POST\">", r.URL.RawQuery)))

	w.Write([]byte("Login: <input type=\"text\" name=\"login\" /><br/>"))
	w.Write([]byte("Password: <input type=\"password\" name=\"password\" /><br/>"))
	w.Write([]byte("<input type=\"submit\"/>"))

	w.Write([]byte("</form>"))

	w.Write([]byte("</body></html>"))
}

func (o *OIDC) AddClient(clientID, clientSecret, redirectUri string) {
	c := &osin.DefaultClient{
		Id:          clientID,
		Secret:      clientSecret,
		RedirectUri: redirectUri,
		UserData:    nil,
	}
	o.memStorage.SetClient(clientID, c)
}

func (o *OIDC) AddUser(clientID string, userInfo *UserInfo) {
	o.usersMu.Lock()
	defer o.usersMu.Unlock()
	o.users[clientID] = append(o.users[clientID], userInfo)
}

func (o *OIDC) validateUser(clientID, username, password string) (*UserInfo, bool) {
	o.usersMu.Lock()
	defer o.usersMu.Unlock()
	userList := o.users[clientID]
	for _, u := range userList {
		if u.Username == username && u.Password == password {
			return u, true
		}
	}
	return nil, false
}

// encodeIDToken serializes and signs an ID Token then adds a field to the token response.
func encodeIDToken(resp *osin.Response, idToken *IDToken, singer jose.Signer) {
	resp.InternalError = func() error {
		payload, err := json.Marshal(idToken)
		if err != nil {
			return fmt.Errorf("failed to marshal token: %v", err)
		}
		jws, err := jwtSigner.Sign(payload)
		if err != nil {
			return fmt.Errorf("failed to sign token: %v", err)
		}
		raw, err := jws.CompactSerialize()
		if err != nil {
			return fmt.Errorf("failed to serialize token: %v", err)
		}
		resp.Output["id_token"] = raw
		return nil
	}()

	// Record errors as internal server errors.
	if resp.InternalError != nil {
		resp.IsError = true
		resp.ErrorId = osin.E_SERVER_ERROR
	}
}

func generateKey() {
	// Load signing key.
	block, _ := pem.Decode(privateKeyBytes)
	if block == nil {
		logs.Error("no private key found")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		logs.Error("failed to parse key: %v", err)
	}

	// Configure jwtSigner and public keys.
	privateKey := &jose.JsonWebKey{
		Key:       key,
		Algorithm: "RS256",
		Use:       "sig",
		KeyID:     "1", // KeyID should use the key thumbprint.
	}

	jwtSigner, err = jose.NewSigner(jose.RS256, privateKey)
	if err != nil {
		log.Fatalf("failed to create jwtSigner: %v", err)
	}
	publicKeys = &jose.JsonWebKeySet{
		Keys: []jose.JsonWebKey{
			jose.JsonWebKey{Key: &key.PublicKey,
				Algorithm: "RS256",
				Use:       "sig",
				KeyID:     "1",
			},
		},
	}

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
