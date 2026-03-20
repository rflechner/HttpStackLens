package http

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
