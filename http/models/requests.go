package models

type Command interface {
	isCommand()
}

type Connect struct {
	HostPort HostPort
	Version  Version
}

type ProxyRequest struct {
	Connect Connect
	Headers []Header
}
