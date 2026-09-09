// Package webui owns the generated dashboard assets and safe static serving primitives.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed assets
var embedded embed.FS

// Assets returns the generated dashboard rooted at index.html.
func Assets() fs.FS {
	assets, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic("webui: embedded asset root is missing")
	}
	return assets
}

// HTTPHandler serves the embedded dashboard with SPA fallback and defensive headers.
func HTTPHandler() http.Handler { return NewHandler(Assets()) }

// NewHandler serves a dashboard filesystem. It is separated for deterministic tests.
func NewHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		setSecurityHeaders(response.Header())
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name, ok := resolveAsset(assets, request.URL.Path)
		if !ok {
			http.NotFound(response, request)
			return
		}
		content, err := fs.ReadFile(assets, name)
		if err != nil {
			http.NotFound(response, request)
			return
		}
		// Fixed release manifest types must not depend on an OS MIME registry.
		contentType := map[string]string{".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8"}[path.Ext(name)]
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		response.Header().Set("Content-Type", contentType)
		if name == "index.html" {
			response.Header().Set("Cache-Control", "no-store")
		} else {
			response.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(response, request, name, time.Time{}, bytes.NewReader(content))
	})
}

func resolveAsset(assets fs.FS, urlPath string) (string, bool) {
	if strings.ContainsRune(urlPath, '\x00') || strings.Contains(urlPath, `\`) {
		return "", false
	}
	for _, segment := range strings.Split(urlPath, "/") {
		if segment == ".." || segment == "." {
			return "", false
		}
	}
	name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(urlPath, "/")), "/")
	if name == "" || name == "." {
		return "index.html", true
	}
	if info, err := fs.Stat(assets, name); err == nil && !info.IsDir() {
		return name, true
	}
	if path.Ext(name) == "" {
		return "index.html", true
	}
	return "", false
}

func setSecurityHeaders(header http.Header) {
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}
