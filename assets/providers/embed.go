// Package providers embeds the provider SVG assets shipped with QuotaDock.
package providers

import "embed"

// SVGFiles contains the local lobe-icons assets. Runtime CDN access is not
// required.
//
//go:embed *.svg
var SVGFiles embed.FS

// Read returns an embedded provider SVG.
func Read(name string) ([]byte, error) {
	return SVGFiles.ReadFile(name)
}
