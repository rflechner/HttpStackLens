package models

type HostPort struct {
	Host string
	Port int
}

type Header struct {
	Name  string
	Value string
}

type Version struct{ Major, Minor int } // HTTP/1.1, HTTP/2.0
