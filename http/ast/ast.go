package ast

type Command interface {
	isCommand()
}

type Connect struct {
	HostPort HostPort
	Version  Version
}

type Version struct{ Major, Minor int } // HTTP/1.1, HTTP/2.0

type HostPort struct {
	Host string
	Port int
}

type Header struct {
	Name  string
	Value string
}

type ProxyRequest struct {
	Connect Connect
	Headers []Header
}
