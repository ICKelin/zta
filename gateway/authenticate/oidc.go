package authenticate

import (
	"encoding/json"
	"fmt"
	"github.com/astaxie/beego/logs"
	"github.com/openshift/osin"
	jose "gopkg.in/square/go-jose.v1"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ Authenticate = &OIDC{}

type OIDCConfig struct {
	Issuer     string
	ListenAddr string
}

type OIDC struct {
	conf   *OIDCConfig
	server *osin.Server

	wellKnown []byte

	publicKeys []byte
	jwtSigner  jose.Signer

	memStorage *MemStorage
	// client_Id -> user list
	usersMu sync.Mutex
	users   map[string][]*UserInfo
}

func NewOIDC(conf *OIDCConfig, jwtSinger jose.Signer, publicKeys *jose.JsonWebKeySet) *OIDC {
	data := map[string]interface{}{
		"issuer":                                conf.Issuer,
		"authorization_endpoint":                conf.Issuer + "/",
		"token_endpoint":                        conf.Issuer + "/token",
		"jwks_uri":                              conf.Issuer + "/publickeys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "email", "profile"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		"claims_supported": []string{
			"email",
		},
	}
	wellKnown, _ := json.Marshal(data)

	publicKeyBytes, _ := json.Marshal(publicKeys)

	memStorage := NewMemStorage()
	return &OIDC{
		conf:       conf,
		jwtSigner:  jwtSinger,
		wellKnown:  wellKnown,
		publicKeys: publicKeyBytes,
		server:     osin.NewServer(osin.NewServerConfig(), memStorage),
		users:      make(map[string][]*UserInfo),
		memStorage: memStorage,
	}
}

func (o *OIDC) Serve() error {
	http.Handle("/", http.FileServer(http.Dir("./web")))

	http.HandleFunc("/.well-known/openid-configuration", o.handleDiscovery)
	http.HandleFunc("/publickeys", o.handlePublicKeys)
	http.HandleFunc("/authorize", o.handleAuthorization)
	http.HandleFunc("/token", o.handleToken)

	return http.ListenAndServe(o.conf.ListenAddr, nil)
}

// handleDiscovery for client (eg: apisix)
func (o *OIDC) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(o.wellKnown)))
	w.Write(o.wellKnown)
}

// handlePublicKeys for client (eg: apisix)
func (o *OIDC) handlePublicKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(o.publicKeys)))
	w.Write(o.publicKeys)
}

// handleAuthorization for user agent (eg: web browser)
func (o *OIDC) handleAuthorization(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	if r.Method == "GET" {
		//loginPage(w, r)
		http.ServeFile(w, r, "./web/index.html")
		return
	}

	resp := o.server.NewResponse()
	defer resp.Close()

	// validate redirect uri
	ar := o.server.HandleAuthorizeRequest(resp, r)
	if ar == nil {
		if resp.InternalError != nil {
			resp.InternalError = fmt.Errorf("get authorize request fail")
		}
		replyToUserAgent(w, nil, resp.InternalError)
		return
	}

	// authenticate
	body := make([]byte, r.ContentLength)
	_, err := io.ReadFull(r.Body, body)
	if err != nil && err != io.EOF {
		replyToUserAgent(w, nil, err)
		return
	}
	userInfo := make(map[string]string)
	err = json.Unmarshal(body, &userInfo)
	if err != nil {
		replyToUserAgent(w, nil, err)
		return
	}

	//user, ok := o.validateUser(ar.Client.GetId(), r.FormValue("username"), r.FormValue("password"))
	user, ok := o.validateUser(ar.Client.GetId(), userInfo["username"], userInfo["password"])
	if !ok {
		replyToUserAgent(w, nil, fmt.Errorf("invalid user"))
		return
	}

	ar.Authorized = true
	scopes := make(map[string]bool)
	for _, s := range strings.Fields(ar.Scope) {
		scopes[s] = true
	}

	if scopes["openid"] {
		now := time.Now()
		idToken := IDToken{
			Issuer:     o.conf.Issuer,
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
	redirectUri, _ := resp.GetRedirectUrl()
	replyToUserAgent(w, map[string]string{
		"redirect_uri": redirectUri,
	}, nil)
}

// handle token for client (eg: apisix)
func (o *OIDC) handleToken(w http.ResponseWriter, r *http.Request) {
	resp := o.server.NewResponse()
	defer resp.Close()

	fmt.Println("request token...")

	ar := o.server.HandleAccessRequest(resp, r)
	if ar == nil {
		logs.Error("handle access request fail: %v", resp.InternalError)
		osin.OutputJSON(resp, w, r)
		return
	}

	idToken, ok := ar.UserData.(*IDToken)
	if !ok {
		resp.IsError = true
		resp.InternalError = fmt.Errorf("invalid id token format")
		osin.OutputJSON(resp, w, r)
		return
	}

	body, err := json.Marshal(idToken)
	if err != nil {
		logs.Error("marshal id token fail: %v", err)
		resp.IsError = true
		resp.ErrorId = osin.E_SERVER_ERROR
		osin.OutputJSON(resp, w, r)
		return
	}

	jws, err := o.jwtSigner.Sign(body)
	if err != nil {
		logs.Error("jwt sign fail: %v", err)
		resp.IsError = true
		resp.ErrorId = osin.E_SERVER_ERROR
		osin.OutputJSON(resp, w, r)
		return
	}

	raw, err := jws.CompactSerialize()
	if err != nil {
		logs.Error("jws compact serialize fail: %v", err)
		resp.IsError = true
		resp.ErrorId = osin.E_SERVER_ERROR
		osin.OutputJSON(resp, w, r)
		return
	}
	ar.Authorized = true
	resp.Output["id_token"] = raw
	o.server.FinishAccessRequest(resp, r, ar)
	osin.OutputJSON(resp, w, r)
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

// AddClient add a new osin.Client to storage
// concurrency safety
func (o *OIDC) AddClient(clientID, clientSecret, redirectUri string) {
	c := &osin.DefaultClient{
		Id:          clientID,
		Secret:      clientSecret,
		RedirectUri: redirectUri,
		UserData:    nil,
	}
	o.memStorage.SetClient(clientID, c)
}

// AddUser add a new UserInfo to storage
// concurrency safety
func (o *OIDC) AddUser(clientID string, userInfo *UserInfo) {
	o.usersMu.Lock()
	defer o.usersMu.Unlock()
	o.users[clientID] = append(o.users[clientID], userInfo)
}

func loginPage(w http.ResponseWriter, r *http.Request) bool {
	w.Write([]byte("<html><body>"))

	w.Write([]byte(fmt.Sprintf("LOGIN  (use test/test)<br/>")))
	w.Write([]byte(fmt.Sprintf("<form action=\"/authorize?%s\" method=\"POST\">", r.URL.RawQuery)))

	w.Write([]byte("Login: <input type=\"text\" name=\"login\" /><br/>"))
	w.Write([]byte("Password: <input type=\"password\" name=\"password\" /><br/>"))
	w.Write([]byte("<input type=\"submit\"/>"))

	w.Write([]byte("</form>"))

	w.Write([]byte("</body></html>"))
	return true
}
