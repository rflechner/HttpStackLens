package http

type Command interface {
	isCommand()
}

type Connect struct {
	Host    string
	Port    int
	Version HTTPVersion
}

type HTTPVersion struct{ Major, Minor int } // HTTP/1.1, HTTP/2.0
