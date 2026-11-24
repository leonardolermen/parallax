// Package exploracao monta uma captura sem tráfego gravado: descobre as rotas
// GET de um serviço que já está rodando (a base) e grava o que ele respondeu,
// no mesmo formato da captura do "parallax record". O replay e o check usam a
// captura como se fosse tráfego real.
//
// As rotas vêm de três lugares: do código-fonte (registros de rota com o
// caminho escrito como texto), das respostas do próprio serviço (links do
// HTML, caminhos citados no JavaScript e no JSON, documento OpenAPI) e de
// modelos com parâmetros ("/pedidos/{id}") preenchidos com valores que
// apareceram nas respostas. Só GET e HEAD; nenhuma chamada a modelo de IA.
package exploracao

import (
	"net/url"
	"regexp"
	"strings"
)

// Segmento é um pedaço do caminho entre barras: um texto fixo ou um
// parâmetro (o nome dele em Parametro).
type Segmento struct {
	Literal   string
	Parametro string
}

func (s Segmento) ehParametro() bool { return s.Parametro != "" }

// Rota é um caminho que pode ter parâmetros, como "/api/projects/{projeto}".
type Rota struct {
	Segmentos []Segmento
	Origem    string
}

// Modelo é a rota escrita com os parâmetros entre chaves.
func (r Rota) Modelo() string {
	var b strings.Builder
	for _, s := range r.Segmentos {
		b.WriteByte('/')
		if s.ehParametro() {
			b.WriteString("{" + s.Parametro + "}")
		} else {
			b.WriteString(s.Literal)
		}
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

// Concreta diz se a rota não tem parâmetros.
func (r Rota) Concreta() bool {
	for _, s := range r.Segmentos {
		if s.ehParametro() {
			return false
		}
	}
	return true
}

// Casa diz se o caminho concreto cabe no modelo.
func (r Rota) Casa(caminho string) bool {
	partes := dividir(caminho)
	if len(partes) != len(r.Segmentos) {
		return false
	}
	for i, s := range r.Segmentos {
		if s.ehParametro() {
			if partes[i] == "" {
				return false
			}
			continue
		}
		if s.Literal != partes[i] {
			return false
		}
	}
	return true
}

// dividir quebra "/a/b/" em ["a", "b", ""]; "/" vira [] (a raiz).
func dividir(caminho string) []string {
	caminho = strings.TrimPrefix(caminho, "/")
	if caminho == "" {
		return nil
	}
	return strings.Split(caminho, "/")
}

var (
	reParamChaves   = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.|:[^}]*)?\}$`)
	reParamPontos   = regexp.MustCompile(`^:([A-Za-z_][A-Za-z0-9_]*)\??$`)
	reParamAngulo   = regexp.MustCompile(`^<(?:[a-z]+:)?([A-Za-z_][A-Za-z0-9_]*)>$`)
	reParamJS       = regexp.MustCompile(`\$\{([^}]*)\}`)
	reIdentificador = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	reLiteralOK     = regexp.MustCompile(`^[A-Za-z0-9._~!$&'()*+,;=:@%-]*$`)
)

// AnalisarModelo lê um caminho que pode ter parâmetros nas notações mais
// comuns: {id} e {id...} (Go, OpenAPI), :id (Express), <id> e <int:id>
// (Flask) e ${expr} (template do JavaScript). Devolve false para o que não
// parece um caminho de rota (espaços, curinga "*", "//").
func AnalisarModelo(texto string) (Rota, bool) {
	if i := strings.IndexAny(texto, "?#"); i >= 0 {
		texto = texto[:i]
	}
	if !strings.HasPrefix(texto, "/") || strings.HasPrefix(texto, "//") || len(texto) > 300 {
		return Rota{}, false
	}
	var r Rota
	for _, parte := range dividir(texto) {
		switch {
		case parte == "{$}": // ServeMux do Go: "só exatamente esta rota"
			parte = ""
		case strings.Contains(parte, "*"):
			return Rota{}, false
		}
		if m := reParamChaves.FindStringSubmatch(parte); m != nil {
			r.Segmentos = append(r.Segmentos, Segmento{Parametro: m[1]})
			continue
		}
		if m := reParamPontos.FindStringSubmatch(parte); m != nil {
			r.Segmentos = append(r.Segmentos, Segmento{Parametro: m[1]})
			continue
		}
		if m := reParamAngulo.FindStringSubmatch(parte); m != nil {
			r.Segmentos = append(r.Segmentos, Segmento{Parametro: m[1]})
			continue
		}
		if m := reParamJS.FindStringSubmatch(parte); m != nil {
			ids := reIdentificador.FindAllString(m[1], -1)
			nome := "valor"
			for i := len(ids) - 1; i >= 0; i-- { // ${encodeURIComponent(p.name)} → name
				if ids[i] != "encodeURIComponent" && ids[i] != "String" {
					nome = ids[i]
					break
				}
			}
			r.Segmentos = append(r.Segmentos, Segmento{Parametro: nome})
			continue
		}
		if strings.ContainsAny(parte, "{}<>") || !reLiteralOK.MatchString(parte) {
			return Rota{}, false
		}
		if dec, err := url.PathUnescape(parte); err == nil {
			parte = dec
		}
		r.Segmentos = append(r.Segmentos, Segmento{Literal: parte})
	}
	return r, true
}
