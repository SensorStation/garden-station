package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

//go:embed app
var tmpldir embed.FS

func (g *Gardener) InitApp() {
	appFS, err := fs.Sub(tmpldir, "app")
	if err != nil {
		slog.Error("failed to sub embed FS", "error", err)
		return
	}

	tmpl, err := template.ParseFS(appFS, "index.html")
	if err != nil {
		slog.Error("failed to parse templates", "error", err)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.Execute(w, nil); err != nil {
			slog.Error("template execution failed", "error", err)
		}
	})
	mux.Handle("/css/", http.FileServer(http.FS(appFS)))
	mux.Handle("/js/", http.FileServer(http.FS(appFS)))

	go func() {
		slog.Info("web server listening", "addr", ":8011")
		if err := http.ListenAndServe(":8011", mux); err != nil {
			slog.Error("web server failed", "error", err)
		}
	}()
}
