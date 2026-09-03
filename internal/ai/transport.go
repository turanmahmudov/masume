package ai

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// Some gateways are behind a network device that resets the connection instead of answering
// a hello that offers TLS 1.3. The handshake fails before the client writes a request, so the
// client opens the connection again with TLS 1.2 as the maximum and stores the host.

// handshakeTimeout is the time limit of one attempt to open a connection.
const handshakeTimeout = 20 * time.Second

// responseHeaderTimeout is the time a provider has to send its headers. The body after them
// has no time limit, because a model can write a long reply.
const responseHeaderTimeout = 2 * time.Minute

// tlsFallback stores the hosts that answer only with TLS 1.2 as the maximum.
type tlsFallback struct {
	guard sync.RWMutex
	hosts map[string]bool
}

// needsOldTLS reports whether this host refused a hello that offered TLS 1.3.
func (fallback *tlsFallback) needsOldTLS(address string) bool {
	fallback.guard.RLock()
	defer fallback.guard.RUnlock()
	return fallback.hosts[address]
}

// keepOldTLS records that this host answers only with TLS 1.2 as the maximum.
func (fallback *tlsFallback) keepOldTLS(address string) {
	fallback.guard.Lock()
	defer fallback.guard.Unlock()
	if fallback.hosts == nil {
		fallback.hosts = map[string]bool{}
	}
	fallback.hosts[address] = true
}

var oldTLSHosts = &tlsFallback{}

// buildTLSConfig returns the configuration of one attempt. It lists the protocols, so the
// transport can still use HTTP/2 if the server supports it.
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

// shakeHands opens one connection and runs its handshake.
func shakeHands(
	ctx context.Context, dialer *net.Dialer, network, address, host string, ceiling uint16,
) (net.Conn, error) {
	// The end of this context does not affect a completed handshake, so the connection
	// stays open after it.
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

// dialTLS opens the connection of one request. If a host resets a hello that offered TLS
// 1.3, the client tries again with TLS 1.2 as the maximum. The client writes no part of the
// request before the handshake, so the second attempt sends nothing a second time.
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
	// A cancelled context is not a host that refuses the hello.
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

// buildTransport returns the transport used by every request of the chat.
func buildTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialTLSContext = dialTLS
	transport.ForceAttemptHTTP2 = true
	transport.TLSHandshakeTimeout = handshakeTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return transport
}
