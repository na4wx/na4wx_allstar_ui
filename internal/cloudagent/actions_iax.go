package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hamvoipconfiggui/internal/config"
)

// Standard AllStarLink registration/peer defaults -- mirrors
// internal/server/nodes.go's own defaultRegistrationHost/defaultPeerType/
// etc. constants exactly, so a node registered through the cloud ends up
// configured identically to one registered through the local app.
const (
	iaxDefaultRegistrationHost = "register.allstarlink.org"
	iaxDefaultPeerType         = "friend"
	iaxDefaultPeerContext      = "radio-secure"
	iaxDefaultPeerHost         = "dynamic"
	iaxDefaultPeerAuth         = "md5"
)

// iaxDefaultIfBlank mirrors internal/server/nodes.go's own
// defaultIfBlank — see that function's doc comment for why blank fields
// are defaulted here rather than left to client-side placeholder text.
func iaxDefaultIfBlank(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

type iaxRegistrationResult struct {
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     string `json:"port"`
}

type iaxPeerResult struct {
	Type    string `json:"type"`
	Context string `json:"context"`
	Host    string `json:"host"`
	Secret  string `json:"secret"`
	Auth    string `json:"auth"`
}

type iaxLoadRegistrationResult struct {
	Registration *iaxRegistrationResult `json:"registration,omitempty"`
	Peer         *iaxPeerResult         `json:"peer,omitempty"`
}

type iaxLoadRegistrationParams struct {
	Number string `json:"number"`
}

// actionIAXLoadRegistration wraps config.Store.LoadRegistration +
// config.Store.LoadPeer, combined into one result — either half may be
// absent if this node has never been registered.
func (a *Agent) actionIAXLoadRegistration(_ context.Context, params json.RawMessage) (any, error) {
	var p iaxLoadRegistrationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	reg, err := a.store.LoadRegistration(p.Number)
	if err != nil {
		return nil, err
	}
	peer, err := a.store.LoadPeer(p.Number)
	if err != nil {
		return nil, err
	}
	var result iaxLoadRegistrationResult
	if reg != nil {
		result.Registration = &iaxRegistrationResult{Password: reg.Password, Host: reg.Host, Port: reg.Port}
	}
	if peer != nil {
		result.Peer = &iaxPeerResult{Type: peer.Type, Context: peer.Context, Host: peer.Host, Secret: peer.Secret, Auth: peer.Auth}
	}
	return result, nil
}

type iaxSaveRegistrationParams struct {
	Number      string `json:"number"`
	Password    string `json:"password"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	PeerType    string `json:"peerType"`
	PeerContext string `json:"peerContext"`
	PeerHost    string `json:"peerHost"`
	PeerSecret  string `json:"peerSecret"`
	PeerAuth    string `json:"peerAuth"`
}

// actionIAXSaveRegistration wraps config.Store.SaveRegistration +
// config.Store.SavePeer, always saving both together — mirroring
// handleNodeRegistrationSave's own "a registration without a matching
// peer leaves the node half-configured" reasoning exactly, including
// applying the same standard defaults when a field is left blank.
func (a *Agent) actionIAXSaveRegistration(_ context.Context, params json.RawMessage) (any, error) {
	var p iaxSaveRegistrationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if strings.TrimSpace(p.Password) == "" {
		return nil, fmt.Errorf("a registration password is required")
	}

	reg := config.Registration{
		Node:     p.Number,
		Password: p.Password,
		Host:     iaxDefaultIfBlank(p.Host, iaxDefaultRegistrationHost),
		Port:     p.Port,
	}
	peer := &config.Peer{
		Node:    p.Number,
		Type:    iaxDefaultIfBlank(p.PeerType, iaxDefaultPeerType),
		Context: iaxDefaultIfBlank(p.PeerContext, iaxDefaultPeerContext),
		Host:    iaxDefaultIfBlank(p.PeerHost, iaxDefaultPeerHost),
		Secret:  iaxDefaultIfBlank(p.PeerSecret, p.Password),
		Auth:    iaxDefaultIfBlank(p.PeerAuth, iaxDefaultPeerAuth),
	}

	if err := a.store.SaveRegistration(reg); err != nil {
		return nil, err
	}
	if err := a.store.SavePeer(peer); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
