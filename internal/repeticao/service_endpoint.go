package repeticao

import (
	"regexp"
	"strings"
)

var identificador = regexp.MustCompile(`^(?:[0-9]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|[0-7][0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{25}|[0-9a-fA-F]{16,})$`)

// Endpoint agrupa numeros, UUIDs, ULIDs e hexadecimais longos na rota.
func Endpoint(metodo, caminho string) string {
	parts := strings.Split(caminho, "/")
	for i, p := range parts {
		if identificador.MatchString(p) {
			parts[i] = "{id}"
		}
	}
	return metodo + " " + strings.Join(parts, "/")
}
