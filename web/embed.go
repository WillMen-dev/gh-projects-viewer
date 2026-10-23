// Package web contiene el frontend (HTML5, CSS3 y JavaScript sin frameworks),
// embebido en el binario de Go para que el despliegue sea un único ejecutable.
package web

import (
	"embed"
	"io/fs"
)

//go:embed public
var content embed.FS

// Static devuelve el sistema de archivos con raíz en web/public.
func Static() fs.FS {
	sub, err := fs.Sub(content, "public")
	if err != nil {
		panic(err)
	}
	return sub
}
