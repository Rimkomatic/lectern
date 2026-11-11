package webui

import "embed"

// dist holds the output of `npm run build` in web/. The directory always
// contains a .gitkeep so the embed directive compiles before the UI is built.
//
//go:embed all:dist
var dist embed.FS
