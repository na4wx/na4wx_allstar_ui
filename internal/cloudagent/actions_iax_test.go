package cloudagent

import (
	"context"
	"encoding/json"
	"testing"
)

func TestActionIAXLoadRegistrationEmpty(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "iax.loadRegistration", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	r, ok := result.(iaxLoadRegistrationResult)
	if !ok {
		t.Fatalf("result type = %T, want iaxLoadRegistrationResult", result)
	}
	if r.Registration != nil || r.Peer != nil {
		t.Fatalf("result = %+v, want both halves absent for a never-registered node", r)
	}
}

func TestActionIAXSaveRegistrationAppliesDefaults(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000", "password": "secretpass"})
	if _, err := a.dispatch(context.Background(), "iax.saveRegistration", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	reg, err := a.store.LoadRegistration("2000")
	if err != nil {
		t.Fatal(err)
	}
	if reg == nil || reg.Password != "secretpass" || reg.Host != iaxDefaultRegistrationHost {
		t.Fatalf("registration = %+v, want defaulted host %q", reg, iaxDefaultRegistrationHost)
	}

	peer, err := a.store.LoadPeer("2000")
	if err != nil {
		t.Fatal(err)
	}
	if peer == nil || peer.Type != iaxDefaultPeerType || peer.Context != iaxDefaultPeerContext || peer.Host != iaxDefaultPeerHost || peer.Auth != iaxDefaultPeerAuth {
		t.Fatalf("peer = %+v, want the standard defaults", peer)
	}
	if peer.Secret != "secretpass" {
		t.Fatalf("peer.Secret = %q, want it defaulted to the registration password", peer.Secret)
	}
}

func TestActionIAXSaveRegistrationHonorsExplicitFields(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{
		"number":      "2000",
		"password":    "secretpass",
		"host":        "custom.example.com",
		"port":        "4570",
		"peerType":    "peer",
		"peerContext": "custom-context",
		"peerHost":    "10.0.0.5",
		"peerSecret":  "differentsecret",
		"peerAuth":    "rsa",
	})
	if _, err := a.dispatch(context.Background(), "iax.saveRegistration", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	reg, err := a.store.LoadRegistration("2000")
	if err != nil {
		t.Fatal(err)
	}
	if reg == nil || reg.Host != "custom.example.com" || reg.Port != "4570" {
		t.Fatalf("registration = %+v, want the explicit host/port honored", reg)
	}

	peer, err := a.store.LoadPeer("2000")
	if err != nil {
		t.Fatal(err)
	}
	if peer == nil || peer.Type != "peer" || peer.Context != "custom-context" || peer.Host != "10.0.0.5" || peer.Secret != "differentsecret" || peer.Auth != "rsa" {
		t.Fatalf("peer = %+v, want every explicit field honored over the defaults", peer)
	}
}

func TestActionIAXSaveRegistrationRejectsBlankPassword(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "iax.saveRegistration", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of a blank registration password")
	}
}

func TestActionIAXLoadRegistrationAfterSave(t *testing.T) {
	a := newConfigTestAgent(t)
	saveParams, _ := json.Marshal(map[string]string{"number": "2000", "password": "secretpass"})
	if _, err := a.dispatch(context.Background(), "iax.saveRegistration", saveParams); err != nil {
		t.Fatalf("save: %v", err)
	}

	loadParams, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "iax.loadRegistration", loadParams)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	r := result.(iaxLoadRegistrationResult)
	if r.Registration == nil || r.Registration.Password != "secretpass" {
		t.Fatalf("registration = %+v", r.Registration)
	}
	if r.Peer == nil || r.Peer.Secret != "secretpass" {
		t.Fatalf("peer = %+v", r.Peer)
	}
}
