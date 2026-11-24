package exploracao

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// Origens de um caminho explorado. Fora os links, todas são adivinhadas (a
// raiz, código, script, modelo, OpenAPI, sonda) e só entram na captura se o
// serviço não respondeu 404/405: quem adivinha erra, e um 404 nas duas
// versões não testa nada.
const (
	OrigemInicio  = "inicio"
	OrigemLink    = "link"
	OrigemCodigo  = "codigo"
	OrigemScript  = "script"
	OrigemOpenAPI = "openapi"
	OrigemModelo  = "modelo"
	OrigemSonda   = "sonda"
)

func adivinhada(origem string) bool {
	return origem != OrigemLink
}

// achado é um caminho (ou modelo) citado numa resposta.
type achado struct {
	texto  string
	origem string
}

var (
	reAtributoHTML = regexp.MustCompile(`(?i)\b(?:href|src)\s*=\s*["']([^"'#\s]+)`)
	reTextoScript  = regexp.MustCompile("[\"'`](/[A-Za-z0-9_\\-./{}$:~%?=&()]*)[\"'`]")
)

// extrairDoHTML devolve os links do HTML e os caminhos citados nos scripts
// embutidos.
func extrairDoHTML(corpo []byte) []achado {
	var out []achado
	for _, m := range reAtributoHTML.FindAllSubmatch(corpo, -1) {
		out = append(out, achado{texto: htmlDesescapar(string(m[1])), origem: OrigemLink})
	}
	return append(out, extrairDoScript(corpo)...)
}

func htmlDesescapar(s string) string {
	return strings.NewReplacer("&amp;", "&", "&#38;", "&", "&#x2F;", "/", "&#47;", "/").Replace(s)
}

// extrairDoScript devolve os textos do JavaScript que parecem caminhos do
// próprio serviço ("/api/x", `/api/x/${id}`).
func extrairDoScript(corpo []byte) []achado {
	var out []achado
	for _, m := range reTextoScript.FindAllSubmatch(corpo, -1) {
		t := string(m[1])
		if t == "/" || strings.HasPrefix(t, "//") || strings.Contains(t, "/*") {
			continue
		}
		out = append(out, achado{texto: t, origem: OrigemScript})
	}
	return out
}

// decodificarJSON devolve o JSON do corpo, se ele for JSON.
func decodificarJSON(corpo []byte) (any, bool) {
	c := bytes.TrimSpace(corpo)
	if len(c) == 0 || (c[0] != '{' && c[0] != '[') {
		return nil, false
	}
	d := json.NewDecoder(bytes.NewReader(c))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

// extrairDoJSON devolve os caminhos citados em valores do JSON (strings que
// começam com "/" ou são URLs absolutas) e, se o JSON for um documento
// OpenAPI/Swagger, as rotas GET dele.
func extrairDoJSON(v any) []achado {
	var out []achado
	if doc, ok := v.(map[string]any); ok {
		out = append(out, rotasOpenAPI(doc)...)
	}
	var andar func(x any, prof int)
	andar = func(x any, prof int) {
		if prof > 8 {
			return
		}
		switch t := x.(type) {
		case map[string]any:
			for _, c := range chavesOrdenadas(t) {
				andar(t[c], prof+1)
			}
		case []any:
			for _, e := range t {
				andar(e, prof+1)
			}
		case string:
			if len(t) <= 300 && !strings.ContainsAny(t, " \t\r\n") &&
				((strings.HasPrefix(t, "/") && !strings.HasPrefix(t, "//")) ||
					strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")) {
				out = append(out, achado{texto: t, origem: OrigemLink})
			}
		}
	}
	andar(v, 0)
	return out
}

func rotasOpenAPI(doc map[string]any) []achado {
	caminhos, ok := doc["paths"].(map[string]any)
	if !ok {
		return nil
	}
	if _, oa := doc["openapi"]; !oa {
		if _, sw := doc["swagger"]; !sw {
			return nil
		}
	}
	prefixo := ""
	if bp, ok := doc["basePath"].(string); ok { // Swagger 2
		prefixo = strings.TrimRight(bp, "/")
	} else if servers, ok := doc["servers"].([]any); ok && len(servers) > 0 { // OpenAPI 3, servidor relativo
		if s, ok := servers[0].(map[string]any); ok {
			if u, ok := s["url"].(string); ok && strings.HasPrefix(u, "/") {
				prefixo = strings.TrimRight(u, "/")
			}
		}
	}
	var out []achado
	for _, p := range chavesOrdenadas(caminhos) {
		ops, ok := caminhos[p].(map[string]any)
		if !ok {
			continue
		}
		if _, temGET := ops["get"]; temGET {
			out = append(out, achado{texto: prefixo + p, origem: OrigemOpenAPI})
		}
	}
	return out
}
