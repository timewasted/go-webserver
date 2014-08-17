// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"crypto/tls"
	"net"
	"sync/atomic"
)

// ErrListenerClosed is an error type that indicates that the listener has
// intentionally been closed.
type ErrListenerClosed struct {
	error
}

// WebServerListener is an implementation of the net.Listener interface that
// handles HTTP and HTTPS requests, while also supporting graceful shutdowns.
type WebServerListener struct {
	net.Listener
	listening int32
}

// Listen creates a WebServerListener. If tlsConfig is nil, an HTTP listener
// is created. Otherwise, an HTTPS listener is created.
func Listen(network, laddr string, tlsConfig *tls.Config) (*WebServerListener, error) {
	var listener net.Listener
	var err error

	if tlsConfig != nil {
		listener, err = tls.Listen(network, laddr, tlsConfig)
	} else {
		listener, err = net.Listen(network, laddr)
	}
	if err != nil {
		return nil, err
	}

	wsl := &WebServerListener{
		Listener:  listener,
		listening: 1,
	}
	return wsl, nil
}

// Listening returns true when listening for connections, and false when not.
func (l *WebServerListener) Listening() bool {
	return atomic.LoadInt32(&l.listening) != 0
}

// Accept implements the Accept() method of the net.Listener interface.
func (l *WebServerListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		if !l.Listening() {
			return nil, ErrListenerClosed{err}
		}
	}

	return conn, err
}

// Close implements the Close() method of the net.Listener interface.
func (l *WebServerListener) Close() error {
	if !l.Listening() {
		return nil
	}

	atomic.StoreInt32(&l.listening, 0)
	return l.Listener.Close()
}
