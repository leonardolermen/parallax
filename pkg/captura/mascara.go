package captura

import (
	"net/http"
	"strings"
)

// Mascara é o valor gravado no lugar de cada valor de cabeçalho sensível.
const Mascara = "***"

// cabecalhosSensiveis são mascarados na captura, sem diferenciar maiúsculas,
// tanto na requisição quanto na resposta.
var cabecalhosSensiveis = []string{"Authorization", "Cookie", "Set-Cookie", "X-Api-Key"}

// Sensivel diz se o cabeçalho é mascarado na captura (sem diferenciar maiúsculas).
func Sensivel(nome string) bool {
	for _, s := range cabecalhosSensiveis {
		if strings.EqualFold(nome, s) {
			return true
		}
	}
	return false
}

// Mascarar devolve uma cópia de h em que cada valor de cabeçalho sensível
// vira Mascara (a quantidade de valores é preservada). Nunca altera h.
// Devolve um Header vazio (não nil) quando h é nil.
func Mascarar(h http.Header) http.Header {
	c := h.Clone()
	if c == nil {
		c = http.Header{}
	}
	for nome, valores := range c {
		if !Sensivel(nome) {
			continue
		}
		novos := make([]string, len(valores))
		for i := range novos {
			novos[i] = Mascara
		}
		c[nome] = novos
	}
	return c
}
