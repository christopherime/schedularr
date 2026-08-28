package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// uiAllowedMethods is the Allow header value newUIHandler's 405 response
// carries -- the UI is static content, so only reads make sense.
const uiAllowedMethods = "GET, HEAD"

// newUIHandler returns the catch-all handler NewRouter installs via
// r.NotFound(...) when Config.UI is set. It serves uiFS as a static site
// with SPA-friendly directory-index resolution:
//
//   - GET/HEAD only; every other method gets a 405 (there is no routing
//     table to consult -- see NewRouter's doc comment on why chi's NotFound
//     fires regardless of method for a path with zero registered routes).
//   - The request path is resolved against uiFS after rejecting any ".."
//     path segment outright (defense in depth alongside path.Clean's own
//     inability to produce a ".." in an already-rooted URL path -- see
//     uiFilePath).
//   - A directory request resolves to that directory's index.html: "/" ->
//     "index.html", "/blocks/" -> "blocks/index.html". A directory request
//     missing its trailing slash ("/blocks") is 301-redirected to the
//     slash form ("/blocks/"), matching net/http.FileServer's own
//     behavior for the same case.
//   - Content-Type is set from the resolved file's extension via
//     http.ServeContent (e.g. ".html" -> "text/html; charset=utf-8",
//     ".js" -> "text/javascript", ...); ServeContent also handles Range/
//     If-Modified-Since and HEAD requests.
//   - A path that doesn't resolve to any file serves 404.html (read once
//     here, at construction, not per-request) with HTTP 404. If uiFS has
//     no 404.html, the response falls back to net/http's plain-text 404.
//
// Every response this handler writes -- 200, 301, 404, 405 alike -- carries
// X-Content-Type-Options: nosniff and Referrer-Policy: same-origin.
func newUIHandler(uiFS fs.FS) http.HandlerFunc {
	notFoundBody, err := fs.ReadFile(uiFS, "404.html")
	has404 := err == nil

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", uiAllowedMethods)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name, ok := uiFilePath(r.URL.Path)
		if !ok {
			serveUINotFound(w, r, notFoundBody, has404)
			return
		}

		f, stat, ok := openUIFile(uiFS, name)
		if !ok {
			serveUINotFound(w, r, notFoundBody, has404)
			return
		}

		if stat.IsDir() {
			_ = f.Close()
			if !strings.HasSuffix(r.URL.Path, "/") {
				// Redirect to "/"+name+"/", not r.URL.Path+"/" -- name has
				// already been through uiFilePath's path.Clean (so it can't
				// contain ".." or doubled slashes), and always starts
				// without a leading slash, so this can never produce an
				// open-redirect-style "//host/..." Location from a raw,
				// unnormalized request path like "//evil.example/blocks".
				// #nosec G710 -- target is server-controlled: "/" + a
				// path.Clean'd, single-leading-slash-guaranteed fs.FS name,
				// always same-origin and never attacker-supplied.
				http.Redirect(w, r, "/"+name+"/", http.StatusMovedPermanently)
				return
			}
			name = path.Join(name, "index.html")
			f, stat, ok = openUIFile(uiFS, name)
			if !ok {
				serveUINotFound(w, r, notFoundBody, has404)
				return
			}
		}
		defer func() { _ = f.Close() }()

		rs, ok := f.(io.ReadSeeker)
		if !ok {
			// Every real fs.FS this handler is ever built with (embed.FS via
			// web.Site(), fstest.MapFS in tests) hands back a seekable file;
			// this only guards against a hypothetical fs.FS whose files
			// don't, so it degrades to the 404 page rather than panicking.
			serveUINotFound(w, r, notFoundBody, has404)
			return
		}

		http.ServeContent(w, r, name, stat.ModTime(), rs)
	}
}

// uiFilePath validates and resolves an incoming request path into an
// fs.FS-relative name (fs.FS keys never start with "/"; the root is ".").
// Any ".." path segment is rejected outright and reported via ok=false --
// defense in depth alongside path.Clean's own inability to produce a ".."
// element in the output for an already-rooted ("/...") path, matching
// net/http's own containsDotDot guard in its file server.
func uiFilePath(urlPath string) (name string, ok bool) {
	for _, segment := range strings.Split(urlPath, "/") {
		if segment == ".." {
			return "", false
		}
	}

	clean := path.Clean(urlPath)
	if clean == "/" || clean == "." {
		return ".", true
	}
	return strings.TrimPrefix(clean, "/"), true
}

// openUIFile opens name in uiFS and stats it, folding both the "no such
// file" and "stat failed" cases into a single ok=false so callers don't
// need to distinguish them -- either way the request gets the 404 page.
// The caller is responsible for closing f when ok is true.
func openUIFile(uiFS fs.FS, name string) (f fs.File, stat fs.FileInfo, ok bool) {
	f, err := uiFS.Open(name)
	if err != nil {
		return nil, nil, false
	}
	stat, err = f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, false
	}
	return f, stat, true
}

// serveUINotFound writes the embedded 404.html (when present) with HTTP
// 404; otherwise it falls back to net/http's plain-text 404, exactly what
// Config.UI == nil left in place before this handler existed.
func serveUINotFound(w http.ResponseWriter, r *http.Request, body []byte, has404 bool) {
	if !has404 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
