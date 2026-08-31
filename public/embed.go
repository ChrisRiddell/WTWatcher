// Package public embeds the frontend user interface assets directly into the Go binary.
// This allows WTWatcher to be distributed as a single self-contained executable without
// external HTML/CSS/JS file dependencies.
package public

import _ "embed"

// IndexHTML contains the raw HTML structure for the dashboard frontend.
//
//go:embed index.html
var IndexHTML []byte

// StylesCSS contains the styling rules and CSS custom properties (theme colors) for the dashboard.
//
//go:embed styles.css
var StylesCSS []byte

// ScriptsJS contains the compiled ES2022 JavaScript bundle generated from ui/scripts.ts.
//
//go:embed scripts.js
var ScriptsJS []byte
