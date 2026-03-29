package main

import "httpStackLens/proxy/middlewares"

type AppContext struct {
	port     int
	pipeline middlewares.Middleware
}
