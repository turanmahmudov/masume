package ai

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// Some gateways sit behind a middlebox that resets the connection rather than answering a
// hello that offers TLS 1.3. The handshake fails before any request is written, so the
// connection is opened again with TLS 1.2 as the ceiling and the host is remembered.

// handshakeTimeout is how long one attempt at opening a connection may take.
const handshakeTimeout = 20 * time.Second

// responseHeaderTimeout is how long a provider may take to answer with its headers. The body
// after them carries no deadline, because a model writes a long reply for as long as it likes.
const responseHeaderTimeout = 2 * time.Minute

// tlsFallback remembers the hosts that answered only with TLS 1.2 as the ceiling.
type tlsFallback struct {
	guard sync.RWMutex
	hosts map[string]bool
}

// needsOldTLS reports whether this host already refused a hello that offered TLS 1.3.
func (fallback *tlsFallback) needsOldTLS(address string) bool {
	fallback.guard.RLock()
	defer fallback.guard.RUnlock()
	return fallback.hosts[address]
}

// keepOldTLS records that this host returns only with TLS 1.2 as the ceiling.
func (fallback *tlsFallback) keepOldTLS(address string) {
	fallback.guard.Lock()
	defer fallback.guard.Unlock()
	if fallback.hosts == nil {
		fallback.hosts = map[string]bool{}
	}
	fallback.hosts[address] = true
}

var oldTLSHosts = &tlsFallback{}

// buildTLSConfig returns what one attempt offers. The protocols are named so the transport
// can still speak HTTP/2 where the server takes it.
func buildTLSConfig(host string, ceiling uint16) *tls.Config {
	config := &tls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS12,
	}
	if ceiling != 0 {
		config.MaxVersion = ceiling
	}
	return config
}

// shakeHands opens one connection and runs the handshake of it.
func shakeHands(
	ctx context.Context, dialer *net.Dialer, network, address, host string, ceiling uint16,
) (net.Conn, error) {
	// A handshake that is complete is not affected by the end of this context, so the
	// connection outlives it.
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	raw, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	held := tls.Client(raw, buildTLSConfig(host, ceiling))
	if err := held.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return held, nil
}

// dialTLS opens the connection of one request. A host that resets a hello which offered
// TLS 1.3 is asked again with TLS 1.2 as the ceiling. Nothing of the request is written
// before the handshake, so the second attempt sends nothing twice.
func dialTLS(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	dialer := &net.Dialer{Timeout: handshakeTimeout}

	if oldTLSHosts.needsOldTLS(address) {
		return shakeHands(ctx, dialer, network, address, host, tls.VersionTLS12)
	}
	conn, err := shakeHands(ctx, dialer, network, address, host, 0)
	if err == nil {
		return conn, nil
	}
	// A context that ended is not a host that refuses the hello.
	if ctx.Err() != nil {
		return nil, err
	}
	held, secondErr := shakeHands(ctx, dialer, network, address, host, tls.VersionTLS12)
	if secondErr != nil {
		return nil, err
	}
	oldTLSHosts.keepOldTLS(address)
	return held, nil
}

// buildTransport returns the transport every request of the chat goes through.
func buildTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialTLSContext = dialTLS
	transport.ForceAttemptHTTP2 = true
	transport.TLSHandshakeTimeout = handshakeTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return transport
}
