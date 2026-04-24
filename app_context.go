package main

import "httpStackLens/proxy/middlewares"

type AppContext struct {
	port      int
	webUiPort int
	pipeline  middlewares.Middleware
}
