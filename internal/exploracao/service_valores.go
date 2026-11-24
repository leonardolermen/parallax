package exploracao

import (
	"encoding/json"
	"sort"
	"strings"
)

// maxValoresPorChave limita quantos valores guardamos por chave.
const maxValoresPorChave = 20

// valores guarda os valores vistos nas respostas JSON para preencher os
// parâmetros dos modelos: por nome de chave ("id", "projectId") e, para cada
// caminho, os identificadores dos itens das listas que ele devolveu
// ("/api/projects" → os "id"/"name" de cada projeto).
type valores struct {
	porChave   map[string][]string
	porCaminho map[string][]string
}

func novosValores() *valores {
	return &valores{porChave: map[string][]string{}, porCaminho: map[string][]string{}}
}

// normalizar junta as grafias da mesma chave: "project_id", "projectId" e
// "ProjectID" viram "projectid".
func normalizar(chave string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(chave))
}

// chavesDeIdentidade são as chaves que costumam identificar um item numa
// lista, na ordem de preferência.
var chavesDeIdentidade = []string{"id", "uuid", "slug", "key", "chave", "name", "nome", "code", "codigo"}

func adicionar(lista []string, v string) []string {
	if len(lista) >= maxValoresPorChave {
		return lista
	}
	for _, x := range lista {
		if x == v {
			return lista
		}
	}
	return append(lista, v)
}

// escalar devolve o valor como texto, se ele servir num segmento de caminho.
func escalar(x any) (string, bool) {
	var s string
	switch t := x.(type) {
	case string:
		s = t
	case json.Number:
		s = t.String()
	default:
		return "", false
	}
	if s == "" || s == "." || s == ".." || len(s) > 100 || strings.ContainsAny(s, "/?#\\ \t\r\n") {
		return "", false
	}
	return s, true
}

// colher guarda os valores do JSON devolvido em caminho.
func (vs *valores) colher(caminho string, v any) {
	var andar func(x any, prof int)
	andar = func(x any, prof int) {
		if prof > 6 {
			return
		}
		switch t := x.(type) {
		case map[string]any:
			for _, c := range chavesOrdenadas(t) {
				if s, ok := escalar(t[c]); ok {
					n := normalizar(c)
					vs.porChave[n] = adicionar(vs.porChave[n], s)
				} else {
					andar(t[c], prof+1)
				}
			}
		case []any:
			for _, e := range t {
				andar(e, prof+1)
			}
		}
	}
	andar(v, 0)

	// os itens da lista devolvida: a lista no topo ou no primeiro campo que é lista
	var itens []any
	switch t := v.(type) {
	case []any:
		itens = t
	case map[string]any:
		for _, c := range chavesOrdenadas(t) {
			if l, ok := t[c].([]any); ok && len(l) > 0 {
				itens = l
				break
			}
		}
	}
	for _, it := range itens {
		if s, ok := escalar(it); ok { // lista de nomes: ["a", "b"]
			vs.porCaminho[caminho] = adicionar(vs.porCaminho[caminho], s)
			continue
		}
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := identidade(obj); ok {
			vs.porCaminho[caminho] = adicionar(vs.porCaminho[caminho], s)
		}
	}
}

// identidade escolhe o campo que identifica o item: o primeiro de
// chavesDeIdentidade e, sem nenhum deles, um campo terminado em "id".
func identidade(obj map[string]any) (string, bool) {
	norm := map[string]any{}
	for c, x := range obj {
		norm[normalizar(c)] = x
	}
	for _, c := range chavesDeIdentidade {
		if s, ok := escalar(norm[c]); ok {
			return s, true
		}
	}
	for _, c := range chavesOrdenadas(norm) {
		if strings.HasSuffix(c, "id") {
			if s, ok := escalar(norm[c]); ok {
				return s, true
			}
		}
	}
	return "", false
}

// candidatos devolve os valores para o parâmetro nome, no caminho prefixo
// (a parte já preenchida antes dele): primeiro os itens que o prefixo
// listou, depois os de uma chave com o mesmo nome do parâmetro.
func (vs *valores) candidatos(prefixo, nome string) []string {
	var out []string
	for _, v := range vs.porCaminho[prefixo] {
		out = adicionar(out, v)
	}
	n := normalizar(nome)
	for _, v := range vs.porChave[n] {
		out = adicionar(out, v)
	}
	if len(out) == 0 && n != "id" && strings.HasSuffix(n, "id") {
		for _, v := range vs.porChave[strings.TrimSuffix(n, "id")] { // {projectId} e "project"
			out = adicionar(out, v)
		}
	}
	return out
}

func chavesOrdenadas[V any](m map[string]V) []string {
	cs := make([]string, 0, len(m))
	for c := range m {
		cs = append(cs, c)
	}
	sort.Strings(cs)
	return cs
}
