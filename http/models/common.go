package models

type ResourceEndpoint struct {
	Host         string
	Port         int
	PathAndQuery string
}

type Header struct {
	Name  string
	Value string
}

type Version struct{ Major, Minor int } // HTTP/1.1, HTTP/2.0
