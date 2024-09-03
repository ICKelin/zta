package authenticate

import (
	"encoding/json"
	"fmt"
	"github.com/astaxie/beego/logs"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/openshift/osin"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ Authenticate = &OIDC{}

var buildInWellknown = &wellKnown{
	ResponseTypesSupported:            []string{"code"},
	SubjectTypesSupported:             []string{"public"},
	IDTokenSigningAlgValuesSupported:  []string{"RS256"},
	ScopesSupported:                   []string{"openid", "email", "profile"},
	TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
	ClaimsSupported: []string{
		"email",
	},
}

type wellKnown struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	JwksUri                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
}

type ClientInfo struct {
	ClientID     string      `json:"client_id"`
	ClientSecret string      `json:"client_secret"`
	RedirectUri  string      `json:"redirect_uri"`
	Users        []*UserInfo `json:"users"`
}

type OIDCConfig struct {
	ID             string        `json:"id"`
	Issuer         string        `json:"issuer"`
	ListenAddr     string        `json:"listen_addr"`
	PrivateKeyFile string        `json:"private_key_file"`
	PublicKeyFile  string        `json:"public_key_file"`
	StaticFolder   string        `json:"static_folder"`
	Clients        []*ClientInfo `json:"clients"`
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

func NewOIDC(rawConf json.RawMessage) (*OIDC, error) {
	var conf = &OIDCConfig{}
	err := json.Unmarshal(rawConf, conf)
	if err != nil {
		return nil, err
	}

	buildInWellknown.Issuer = conf.Issuer
	buildInWellknown.AuthorizationEndpoint = conf.Issuer + "/"
	buildInWellknown.TokenEndpoint = conf.Issuer + "/token"
	buildInWellknown.JwksUri = conf.Issuer + "/publickeys"
	wellKnownBytes, _ := json.Marshal(buildInWellknown)

	// jwt signer
	signer, publicKeys, err := loadJws(conf.PrivateKeyFile, conf.PublicKeyFile)
	if err != nil {
		return nil, err
	}

	memStorage := NewMemStorage()
	oidc := &OIDC{
		conf:       conf,
		jwtSigner:  signer,
		wellKnown:  wellKnownBytes,
		publicKeys: publicKeys,
		server:     osin.NewServer(osin.NewServerConfig(), memStorage),
		users:      make(map[string][]*UserInfo),
		memStorage: memStorage,
	}

	// initial clients
	for _, client := range conf.Clients {
		oidc.AddClient(client.ClientID, client.ClientSecret, client.RedirectUri)
		// initial client users
		for _, user := range client.Users {
			oidc.AddUser(client.ClientID, user)
		}
	}
	return oidc, nil
}

func (o *OIDC) Serve() error {
	http.Handle("/", http.FileServer(http.Dir(o.conf.StaticFolder)))

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

	resp := o.server.NewResponse()
	defer resp.Close()

	// validate redirect uri
	ar := o.server.HandleAuthorizeRequest(resp, r)
	if ar == nil {
		if resp.InternalError != nil {
			resp.InternalError = fmt.Errorf("get authorize request fail: %v", resp.InternalError)
		} else {
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
