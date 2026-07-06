package main

import (
	"httpStackLens/configuration"
	"net"
	"testing"
)

func TestProxyServerAllowsClientUsesAccessControl(t *testing.T) {
	server := ProxyServer{
		accessControl: configuration.NewAccessControlSettingsStore(configuration.AccessControlSettings{
			Proxy: configuration.AccessControlConfig{Mode: configuration.AccessControlLan},
		}),
	}

	if !server.allowsClient(&net.TCPAddr{IP: net.ParseIP("192.168.1.42"), Port: 50000}) {
		t.Fatal("LAN access should allow private proxy client")
	}
	if server.allowsClient(&net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 50000}) {
		t.Fatal("LAN access should reject public proxy client")
	}
}
