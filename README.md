go-webserver
============

A simple and easy to use HTTP/HTTPS web server which is designed to be a replacement for Go's `http.ListenAndServe`/`http.ListenAndServeTLS` methods. It uses standard library features that were added in Go v1.3 to allow for graceful shutdowns that don't drop any currently active requests.

Usage is simple:

```
import(
	"github.com/timewasted/go-webserver"
	// Your other imports here
	// ...
)

server := webserver.New()
// You still have access to http.Server via server.Server. The one exception
// is that http.Server.ConnState must be set on server.ConnState. This is a
// side effect of how graceful shutdown is performed.

// TLS certificates must be added before calling Listen(). You can skip adding
// certificates if you want to use plain HTTP as opposed to HTTPS.
if err := server.AddTLSCertificateFromFile("/path/to/server.cert", "/path/to/server.key"); err != nil {
	log.Fatal("AddTLSCertificateFromFile error:", err)
}

if err := server.Listen(":44380"); err != nil {
	log.Fatal("Listen error:", err)
}
defer server.Shutdown()

// Serve() does not block. If Listen() has not been called, it immediately
// returns an error. Otherwise, it starts a goroutine to serve connections,
// then returns once the goroutine has started running. Errors from serving
// are passed over the provided error channel.
errChan := make(chan error)
if err := server.Serve(errChan); err != nil {
	log.Fatal("Serve error:", err)
}

// Your application logic here.
// ...

// Wait for shutdown.
err := <-errChan
if _, graceful := err.(webserver.ErrGracefulShutdown); !graceful {
	log.Println("Non-graceful shutdown error:", err)
}
```

License:
--------
```
Copyright (c) 2014, Ryan Rogers
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met: 

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer. 
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution. 

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
