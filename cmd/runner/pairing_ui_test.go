package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPairingPageExplainsBrowserFlow(t *testing.T) {
	handler := newPairingHandler("http://192.168.1.20:8080", "Research-PC", "nonce-1", func(context.Context, pairingInput) (pairingResult, error) {
		t.Fatal("enroll should not run on GET")
		return pairingResult{}, nil
	}, nil)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{"连接这台电脑", "http://192.168.1.20:8080", "Research-PC", "一次性设备配对码", "127.0.0.1"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("GET body missing %q: %s", expected, body)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("security headers=%v", response.Header())
	}
}

func TestPairingPageRejectsInvalidInputWithoutEnrollment(t *testing.T) {
	called := false
	handler := newPairingHandler("", "Research-PC", "nonce-1", func(context.Context, pairingInput) (pairingResult, error) {
		called = true
		return pairingResult{}, nil
	}, nil)
	form := url.Values{"nonce": {"wrong"}, "hub_url": {"not-a-url"}, "invite_code": {"secret"}, "device_name": {"Research-PC"}}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("POST status=%d called=%v", response.Code, called)
	}
	if !strings.Contains(response.Body.String(), "配对页面已失效") {
		t.Fatalf("POST body=%s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatal("pairing page reflected the one-time code")
	}
}

func TestPairingPageEnrollsOnceAndSignalsSuccess(t *testing.T) {
	var received pairingInput
	paired := make(chan pairingResult, 1)
	handler := newPairingHandler("", "Research-PC", "nonce-1", func(_ context.Context, input pairingInput) (pairingResult, error) {
		received = input
		return pairingResult{RunnerID: "runner-1", DeviceID: "device-1", AgentCount: 4}, nil
	}, func(result pairingResult) { paired <- result })
	form := url.Values{"nonce": {"nonce-1"}, "hub_url": {"http://192.168.1.20:8080"}, "invite_code": {"one-time-secret"}, "device_name": {"Research-PC"}}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", response.Code, response.Body.String())
	}
	if received.HubURL != "http://192.168.1.20:8080" || received.InviteCode != "one-time-secret" || received.DeviceName != "Research-PC" {
		t.Fatalf("received=%#v", received)
	}
	result := <-paired
	if result.AgentCount != 4 || !strings.Contains(response.Body.String(), "已检测并登记 4 个本机 Agent 候选") {
		t.Fatalf("result=%#v body=%s", result, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "one-time-secret") {
		t.Fatal("success page reflected the one-time code")
	}
}

func TestValidatePairingInput(t *testing.T) {
	valid := pairingInput{HubURL: "https://hub.example:8443", InviteCode: "code", DeviceName: "PC"}
	if err := validatePairingInput(valid); err != nil {
		t.Fatal(err)
	}
	for _, input := range []pairingInput{
		{HubURL: "hub.example", InviteCode: "code", DeviceName: "PC"},
		{HubURL: "file:///tmp/hub", InviteCode: "code", DeviceName: "PC"},
		{HubURL: "https://user@hub.example", InviteCode: "code", DeviceName: "PC"},
		{HubURL: "https://hub.example", InviteCode: "", DeviceName: "PC"},
		{HubURL: "https://hub.example", InviteCode: "code", DeviceName: ""},
	} {
		if err := validatePairingInput(input); err == nil {
			t.Fatalf("validatePairingInput(%#v) succeeded", input)
		}
	}
}
