package web

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed manifest.json
var ManifestJSON []byte

//go:embed assetlinks.json
var AssetLinksJSON []byte
