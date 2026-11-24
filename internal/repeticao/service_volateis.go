package repeticao

import (
	"strings"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// volatil diz se o caminho JSON está dentro de um dos campos voláteis.
func volatil(caminho string, volateis []string) bool {
	for _, v := range volateis {
		if caminho == v || strings.HasPrefix(caminho, v+".") || strings.HasPrefix(caminho, v+"[") {
			return true
		}
	}
	return false
}

// corpoVolatil diz se o corpo inteiro muda sozinho ("$").
func corpoVolatil(volateis []string) bool {
	for _, v := range volateis {
		if v == "$" {
			return true
		}
	}
	return false
}

// semVolateis tira das mudanças do corpo (a partir de inicio; as anteriores
// são de cabeçalho) as que caem em campos voláteis e recalcula se o contrato
// quebrou com as que sobraram.
func semVolateis(diffs []veredito.Mudanca, inicio int, volateis []string, quebrou bool) ([]veredito.Mudanca, bool) {
	out := append([]veredito.Mudanca(nil), diffs[:inicio]...)
	removeu := false
	for _, d := range diffs[inicio:] {
		if volatil(d.Caminho, volateis) {
			removeu = true
			continue
		}
		out = append(out, d)
	}
	if !removeu {
		return diffs, quebrou
	}
	quebrou = false
	for _, d := range out[inicio:] {
		if quebraContrato(d) {
			quebrou = true
		}
	}
	return out, quebrou
}

// quebraContrato repete a regra do diffJSON sobre uma mudança: o campo sumiu
// ou mudou de tipo.
func quebraContrato(d veredito.Mudanca) bool {
	if len(d.Antes) == 0 {
		return false
	}
	if len(d.Depois) == 0 {
		return true
	}
	return tipoJSON(d.Antes) != tipoJSON(d.Depois)
}

func tipoJSON(raw []byte) byte {
	switch c := raw[0]; c {
	case '{', '[', '"', 'n':
		return c
	case 't', 'f':
		return 'b'
	default:
		return '0'
	}
}
