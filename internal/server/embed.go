package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

// AssetHandler returns an http.Handler that serves the embedded static files.
func AssetHandler() http.Handler {
	// webFS root is "web", so we create a sub filesystem to serve the contents directly.
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("failed to create sub filesystem for embedded assets: " + err.Error())
	}
	return http.FileServer(http.FS(subFS))
}
