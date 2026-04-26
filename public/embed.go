package public

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed styles.css
var StylesCSS []byte

//go:embed scripts.js
var ScriptsJS []byte
