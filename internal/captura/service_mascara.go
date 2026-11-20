package captura

import (
	"net/http"

	pub "github.com/leonardolermen/parallax/pkg/captura"
)

// Mascara é o valor gravado no lugar de cada valor de cabeçalho sensível.
const Mascara = pub.Mascara

func sensivel(nome string) bool { return pub.Sensivel(nome) }

func mascarar(h http.Header) http.Header { return pub.Mascarar(h) }
