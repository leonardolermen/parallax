package repeticao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func falha(r resposta) bool {
	return r.Status >= 500 || r.Erro == "timeout" || r.Erro == "conexao_recusada"
}
func comparar(t captura.Troca, c Config, b, a resposta) []veredito.Achado {
	makeAchado := func(tp veredito.Tipo, o veredito.Origem, d []veredito.Mudanca) []veredito.Achado {
		return []veredito.Achado{{Tipo: tp, Origem: o, Endpoint: Endpoint(t.Metodo, t.Caminho), Reproducao: curl(t, c), Base: b.Lado, Candidata: a.Lado, Diff: d}}
	}
	if falha(a) {
		o := veredito.Branch
		if falha(b) {
			o = veredito.PreExistente
		}
		return makeAchado(veredito.ErroNovo, o, nil)
	}
	if b.Incompleta || a.Incompleta {
		return makeAchado(veredito.Diferenca, veredito.Branch, nil)
	}
	if b.Status != a.Status {
		return makeAchado(veredito.Diferenca, veredito.Branch, nil)
	}
	changed := b.Status != a.Status || b.Erro != a.Erro
	var diffs []veredito.Mudanca
	for _, h := range []string{"Content-Type", "Location", "Cache-Control"} {
		if !reflect.DeepEqual(b.Cabecalhos.Values(h), a.Cabecalhos.Values(h)) {
			changed = true
			before, _ := json.Marshal(b.Cabecalhos.Values(h))
			after, _ := json.Marshal(a.Cabecalhos.Values(h))
			diffs = append(diffs, veredito.Mudanca{Caminho: "$.cabecalhos_resposta." + h, Antes: before, Depois: after})
		}
	}
	broken := false
	bj, bok := decodificar(b.Corpo)
	aj, aok := decodificar(a.Corpo)
	if bok && aok {
		diffJSON("$", bj, aj, true, true, &diffs, &broken)
		changed = changed || len(diffs) > 0
	} else {
		changed = changed || !bytes.Equal(b.Corpo, a.Corpo)
	}
	if !changed {
		return nil
	}
	tp := veredito.Diferenca
	if broken {
		tp = veredito.ContratoQuebrado
	}
	return makeAchado(tp, veredito.Branch, diffs)
}
func decodificar(b []byte) (any, bool) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, false
	}
	return v, true
}
func diffJSON(path string, b, a any, bexists, aexists bool, d *[]veredito.Mudanca, broken *bool) {
	add := func() {
		if len(*d) >= 50 {
			return
		}
		m := veredito.Mudanca{Caminho: path}
		if bexists {
			m.Antes, _ = json.Marshal(b)
		}
		if aexists {
			m.Depois, _ = json.Marshal(a)
		}
		*d = append(*d, m)
	}
	if !bexists || !aexists {
		if bexists {
			*broken = true
		}
		add()
		return
	}
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		*broken = true
		add()
		return
	}
	switch bv := b.(type) {
	case map[string]any:
		av := a.(map[string]any)
		keys := map[string]bool{}
		for k := range bv {
			keys[k] = true
		}
		for k := range av {
			keys[k] = true
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			v, be := bv[k]
			w, ae := av[k]
			diffJSON(path+"."+k, v, w, be, ae, d, broken)
		}
	case []any:
		av := a.([]any)
		n := len(bv)
		if len(av) > n {
			n = len(av)
		}
		for i := 0; i < n; i++ {
			var v, w any
			if i < len(bv) {
				v = bv[i]
			}
			if i < len(av) {
				w = av[i]
			}
			diffJSON(fmt.Sprintf("%s[%d]", path, i), v, w, i < len(bv), i < len(av), d, broken)
		}
	default:
		if !reflect.DeepEqual(b, a) {
			add()
		}
	}
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func curl(t captura.Troca, c Config) string {
	s := "curl -sS -X " + shellQuote(t.Metodo) + " " + shellQuote(urlTroca(c.Candidata, t))
	h := headers(t, c)
	ks := make([]string, 0, len(h))
	for k := range h {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		for _, v := range h[k] {
			s += " -H " + shellQuote(k+": "+v)
		}
	}
	if len(t.CorpoRequisicao) > 0 {
		s += " --data-binary " + shellQuote(string(t.CorpoRequisicao))
	}
	return s
}
