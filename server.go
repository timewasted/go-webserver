// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package webserver provides an easy to use HTTP/HTTPS web server.
package webserver

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
)

var (
	errListenerAlreadyCreated = errors.New("A listener has already been created.")
	errListenerNotYetCreated  = errors.New("A listener has not yet been created.")
)

// ErrGracefulShutdown is an error type that indicates that a graceful
// shutdown has been performed.
type ErrGracefulShutdown struct {
	error
}

// WebServer is a simple HTTP/HTTPS server.
type WebServer struct {
	Server    *http.Server
	ConnState func(net.Conn, http.ConnState)
	listener  *WebServerListener
	wg        sync.WaitGroup
	idleConns map[net.Conn]struct{}
}

// New creates a new WebServer.
func New() *WebServer {
	return &WebServer{
		Server: &http.Server{
			TLSConfig: nil,
		},
		listener:  &WebServerListener{},
		idleConns: map[net.Conn]struct{}{},
	}
}

// AddTLSCertificate reads the certificate and private key from the provided
// PEM blocks, and adds the certificate to the list of certificates that the
// server can use.
func (s *WebServer) AddTLSCertificate(certPEMBlock, keyPEMBlock []byte) error {
	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return err
	}

	s.addTLSCert(cert)
	return nil
}

// AddTLSCertificateFromFile reads the certificate and private key from the
// provided file paths, and adds the certificate to the list of certificates
// that the server can use.
func (s *WebServer) AddTLSCertificateFromFile(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	s.addTLSCert(cert)
	return nil
}

// addTLSCert adds the provided certificate to the list of certificates that
// the server can use.
func (s *WebServer) addTLSCert(cert tls.Certificate) {
	if s.Server.TLSConfig == nil {
		s.Server.TLSConfig = s.DefaultTLSConfiguration()
	}
	s.Server.TLSConfig.Certificates = append(s.Server.TLSConfig.Certificates, cert)
	s.Server.TLSConfig.BuildNameToCertificate()
}

// DefaultTLSConfiguration returns a base TLS configuration that can then be
// customized to fit the needs of the individual server.
func (s *WebServer) DefaultTLSConfiguration() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{},
		NextProtos:   []string{"http/1.1"},
		// Reasoning behind the cipher suite ordering:
		//
		// - The first priority is forward secrecy, so we will prefer to use
		//   ECDHE over RSA for the key exchange.
		// - Of the available ciphers, only AES-GCM is free of known attacks,
		//   so we will want to use that if possible.
		// - Go's CBC-mode ciphers are vulnerable to timing attacks. The only
		//   other alternative would be RC4, which should be considered broken
		//   at this point.
		// - As explained above, 3DES is a better last resort than RC4.
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		},
		PreferServerCipherSuites: true,
		SessionTicketsDisabled:   false,
	}
}

// Listen begins listening on the given address.
func (s *WebServer) Listen(addr string) error {
	if s.listener.Listening() {
		return errListenerAlreadyCreated
	}

	listener, err := Listen("tcp", addr, s.Server.TLSConfig)
	if err != nil {
		return err
	}

	s.listener = listener
	s.Server.Addr = addr
	return nil
}

// Serve begins serving connections.
func (s *WebServer) Serve(errChan chan<- error) error {
	if !s.listener.Listening() {
		return errListenerNotYetCreated
	}

	servingChan := make(chan struct{})
	go func() {
		s.Server.ConnState = s.connStateTracker
		close(servingChan)
		err := s.Server.Serve(s.listener)
		// Serve() can't return an err of nil, so no need to check for it.
		if _, graceful := err.(ErrListenerClosed); graceful {
			s.wg.Wait()
			err = ErrGracefulShutdown{err}
		}
		errChan <- err
	}()

	<-servingChan
	return nil
}

// Shutdown stops serving connections.
func (s *WebServer) Shutdown() {
	if !s.listener.Listening() {
		return
	}

	s.Server.SetKeepAlivesEnabled(false)
	s.listener.Close()
}

// RoutineStarted informs the server that a request handler has started a
// goroutine, and that the server should wait for it to be finished before
// completing a graceful shutdown.
func (s *WebServer) RoutineStarted() {
	s.wg.Add(1)
}

// RoutineCompleted informs the server that a goroutine running inside of a
// request handler has finished running.
func (s *WebServer) RoutineFinished() {
	s.wg.Done()
}

// connStateTracker tracks the state of connections in order to provide
// graceful shutdowns.
func (s *WebServer) connStateTracker(c net.Conn, state http.ConnState) {
	// Connections that have keepalives ENABLED typically transition between
	// states as follows:
	//   new -> active -> idle -> (eventually) closed
	//
	// Connections that have keepalives DISABLED typically transition between
	// states as follows:
	//   new -> active -> closed
	//
	// In order to provide a graceful shutdown, we need to keep track of
	// connections that have entered the idle state. The first time it enters
	// the idle state, the WaitGroup counter is decreased by two instead of
	// only one to account for the delay in reaching the closed state.
	//
	// This also means that connections which have entered the idle state
	// should not decrease the WaitGroup counter when it enters the closed
	// state.
	switch state {
	case http.StateNew, http.StateActive:
		s.wg.Add(1)
	case http.StateIdle:
		if _, idle := s.idleConns[c]; !idle {
			s.wg.Done()
		}
		s.idleConns[c] = struct{}{}
		s.wg.Done()
	case http.StateHijacked, http.StateClosed:
		if _, idle := s.idleConns[c]; idle {
			delete(s.idleConns, c)
		} else {
			s.wg.Add(-2)
		}
	}

	// Notify the user's state tracker, if they defined one.
	if s.ConnState != nil {
		s.ConnState(c, state)
	}
}
