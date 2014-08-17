// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

// Server configuration.
const (
	listenAddr = "127.0.0.1:44380"
	certFile   = "./test/srv1.localhost.crt"
	keyFile    = "./test/srv1.localhost.key"
)

var (
	errChan         = make(chan error)
	longRunningChan = make(chan struct{})
)

// Client configuration.
const (
	caCertFile = "./test/GoTestingCA.crt"
)

var (
	httpTransport = &http.Transport{
		TLSClientConfig: &tls.Config{},
	}
	httpClient = &http.Client{
		Transport: httpTransport,
	}
)

// Route configuration.
const (
	simpleRoute      = "/simple"
	longRunningRoute = "/longrunning"
)

func init() {
	// Trust the provided CA cert.
	caCert, err := ioutil.ReadFile(caCertFile)
	if err != nil {
		panic("Failed to read CA cert file.")
	}
	trustedCAs := x509.NewCertPool()
	if !trustedCAs.AppendCertsFromPEM(caCert) {
		panic("Failed to add CA cert to trusted cert pool.")
	}
	httpTransport.TLSClientConfig.RootCAs = trustedCAs
}

func newServer() *WebServer {
	server := New()

	serveMux := http.NewServeMux()
	serveMux.HandleFunc(simpleRoute, simpleRouteHandler)
	serveMux.HandleFunc(longRunningRoute, longRunningRouteHandler)

	server.Server.Handler = serveMux
	return server
}

func TestBasicServerHTTP(t *testing.T) {
	server := newServer()
	useTLS := false

	// First test with keepalives enabled.
	testBasicServer(t, server, useTLS, true)
	// Then test with keepalives disabled.
	testBasicServer(t, server, useTLS, false)
}

func TestBasicServerHTTPS(t *testing.T) {
	server := newServer()
	useTLS := true

	if err := server.AddTLSCertificateFromFile(certFile, keyFile); err != nil {
		t.Fatalf("Expected AddTLSCertificateFromFile to succeed, received error '%v'.", err)
	}

	// First test with keepalives enabled.
	testBasicServer(t, server, useTLS, true)
	// Then test with keepalives disabled.
	testBasicServer(t, server, useTLS, false)
}

func testBasicServer(t *testing.T, server *WebServer, useTLS, useKeepAlives bool) {
	server.Server.SetKeepAlivesEnabled(useKeepAlives)

	if err := server.Listen(listenAddr); err != nil {
		t.Fatalf("Expected Listen to succeed, received error '%v'.", err)
	}
	defer server.Shutdown()

	if err := server.Serve(errChan); err != nil {
		t.Fatalf("Expected Serve to succeed, received error '%v'.", err)
	}

	// Ensure that connections are being served.
	reqParams := requestParams{
		tls:           useTLS,
		addr:          listenAddr,
		serverName:    "127.0.0.1",
		route:         simpleRoute,
		expectSuccess: true,
	}
	if err := testRequest(reqParams); err != nil {
		t.Fatal(err)
	}

	server.Shutdown()

	// Ensure that connections are no longer being served.
	reqParams.expectSuccess = false
	if err := testRequest(reqParams); err != nil {
		t.Fatal(err)
	}

	err := <-errChan
	if _, graceful := err.(ErrGracefulShutdown); !graceful {
		t.Fatalf("Expected error %T, received error %T.", ErrGracefulShutdown{}, err)
	}
}

func TestGracefulShutdown(t *testing.T) {
	server := newServer()

	if err := server.Listen(listenAddr); err != nil {
		t.Fatalf("Expected Listen to succeed, received error '%v'.", err)
	}
	defer server.Shutdown()

	if err := server.Serve(errChan); err != nil {
		t.Fatalf("Expected Serve to succeed, received error '%v'.", err)
	}

	reqParams := requestParams{
		tls:           false,
		addr:          listenAddr,
		route:         longRunningRoute,
		expectSuccess: true,
	}
	taskCompleted := false

	// Start a long running request.
	server.RoutineStarted()
	go func() {
		defer server.RoutineFinished()
		if err := testRequest(reqParams); err != nil {
			t.Fatal(err)
		}
		taskCompleted = true
	}()

	<-longRunningChan
	server.Shutdown()

	// While the long running request is running, the server should not be
	// accepting more connections.
	reqParams.route = simpleRoute
	reqParams.expectSuccess = false
	if err := testRequest(reqParams); err != nil {
		t.Fatal(err)
	}

	err := <-errChan
	if _, graceful := err.(ErrGracefulShutdown); !graceful {
		t.Fatalf("Expected error %T, received error %T.", ErrGracefulShutdown{}, err)
	}

	if !taskCompleted {
		t.Fatal("Expected task to be completed.")
	}
}

func simpleRouteHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, "Success")
}

func longRunningRouteHandler(w http.ResponseWriter, req *http.Request) {
	longRunningChan <- struct{}{}
	time.Sleep(2 * time.Second)
	fmt.Fprintln(w, "Success")
}

type requestParams struct {
	tls                     bool
	addr, serverName, route string
	expectSuccess           bool
}

// testRequest makes a request using the provided parameters.
func testRequest(p requestParams) error {
	var url string
	if p.tls {
		httpTransport.TLSClientConfig.ServerName = p.serverName
		url = "https://" + p.addr + p.route
	} else {
		url = "http://" + p.addr + p.route
	}
	resp, err := httpClient.Get(url)
	if err == nil {
		resp.Body.Close()
	}

	if p.expectSuccess {
		if err != nil {
			return fmt.Errorf("Expected request to %s to succeed, received error '%v'.", url, err)
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("Expected request to %s to return status code 200, received %v.", url, resp.StatusCode)
		}
	} else {
		if err == nil {
			return fmt.Errorf("Expected request to %s to fail, received success.", url)
		}
	}

	return nil
}
