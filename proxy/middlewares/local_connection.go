package middlewares

import "net"

// LocalConn marks a connection created inside the application — today, the Web
// UI composer sending a request through the pipeline — as opposed to one
// accepted from a proxy client.
//
// Middlewares that authenticate the client to this proxy skip such connections:
// the browser-facing 407 challenge exists to establish who is using the proxy,
// and here the caller is the application itself. Everything below that layer
// (no_proxy routing, upstream forwarding with its own authentication, HTTPS
// decryption) still applies, so a composer request is handled exactly like a
// browser request otherwise.
type LocalConn struct {
	net.Conn
}

// Local identifies the connection as application-issued. It is an interface
// method rather than a type assertion on LocalConn so that wrappers — the
// buffered NetworkStream, chiefly — can forward the marker down the pipeline.
func (LocalConn) Local() bool { return true }

// IsLocal reports whether a connection was created inside the application.
func IsLocal(connection net.Conn) bool {
	marker, ok := connection.(interface{ Local() bool })
	return ok && marker.Local()
}
