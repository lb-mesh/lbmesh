// Package webdist exposes the compiled React SPA as an embedded filesystem so
// that pkg/dashboard can serve it without any external file-system dependency.
// This file lives at the module root (the direct parent of web/) so that the
// //go:embed directive can reference web/dist with no ".." traversal.
package webdist

import "embed"

// FS holds the contents of web/dist as compiled by `npm run build` inside the
// /web workspace.  All static assets (HTML, JS bundles, CSS, icons) are
// embedded into the binary at compile time.
//
//go:embed all:web/dist
var FS embed.FS

// Root is the sub-path within FS where the dist files are rooted.
// http.FS(FS) would expose paths as "web/dist/…"; the dashboard server calls
// fs.Sub(FS, Root) to strip the prefix and serve "/" directly.
const Root = "web/dist"
