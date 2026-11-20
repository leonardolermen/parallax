package julgamento

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func execucaoTeste(v veredito.Veredito) Execucao {
	return Execucao{ID: v.ID, Data: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC), Captura: "/tmp/c.jsonl",
		Base: "http://base", Candidata: "http://cand", Resultado: v.Resultado, Resumo: v.Resumo}
}

func laudo(t *testing.T, r repeticao.Resultado) string {
	t.Helper()
	v, err := Julgar(r)
	if err != nil {
		t.Fatal(err)
	}
	e := execucaoTeste(v)
	e.Estatisticas = r.Estatisticas
	var b strings.Builder
	if err := EscreverLaudo(&b, e, v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestLaudoCompleto(t *testing.T) {
	lat := achado(veredito.Latencia, veredito.Branch, "GET /lento")
	lat.Base.LatenciaMs, lat.Candidata.LatenciaMs = 40, 100
	dif := achado(veredito.Diferenca, veredito.Branch, "GET /b")
	dif.Base.Status, dif.Candidata.Status = 200, 200
	dif.Reproducao = "curl -X GET 'http://cand/b'"
	for i := 0; i < 12; i++ {
		dif.Diff = append(dif.Diff, veredito.Mudanca{Caminho: fmt.Sprintf("$.itens[%d]", i), Antes: json.RawMessage("1"), Depois: json.RawMessage("2")})
	}
	inc := achado(veredito.Diferenca, veredito.Branch, "GET /a")
	inc.Candidata.Corpo = "xxx" + repeticao.MarcadorIncompleta
	r := resultadoTeste(4, lat, dif, inc)
	r.Estatisticas.Incompletas = 1
	s := laudo(t, r)
	for _, esperado := range []string{
		"# Laudo Parallax " + idTeste, "2026-11-19T12:00:00Z", "/tmp/c.jsonl", "http://base", "http://cand",
		"## Veredito", "REGRESSAO", "## Estatísticas", "p95", "Puladas | 3", "Incompletas | 1",
		"## Regressões de latência", "| GET /lento | 40.0 | 100.0 | +150.0% | branch |",
		"## Diferenças por endpoint", "### GET /a", "### GET /b", "`$.itens[9]`: 1 → 2", "e mais 2 mudanças",
		"curl -X GET 'http://cand/b'",
		"## Comparações incompletas", "- GET /a: candidata",
	} {
		if !strings.Contains(s, esperado) {
			t.Errorf("laudo sem %q:\n%s", esperado, s)
		}
	}
	if strings.Contains(s, "$.itens[10]") || strings.Contains(s, "### GET /lento") {
		t.Errorf("laudo com itens demais ou latência repetida nas diferenças:\n%s", s)
	}
	if strings.Index(s, "### GET /a") > strings.Index(s, "### GET /b") {
		t.Error("endpoints fora de ordem alfabética")
	}
}

func TestLaudoSecoesVazias(t *testing.T) {
	s := laudo(t, resultadoTeste(2))
	if strings.Count(s, "Nenhuma.") != 3 || !strings.Contains(s, "APROVADO") {
		t.Fatalf("laudo aprovado:\n%s", s)
	}
}

func TestLaudoCercaTabelaELimitePorEndpoint(t *testing.T) {
	lat := achado(veredito.Latencia, veredito.Branch, "GET /a|b")
	lat.Base.LatenciaMs, lat.Candidata.LatenciaMs = 10, 50
	cerca := achado(veredito.Diferenca, veredito.Branch, "POST /c")
	cerca.Reproducao = "curl --data-binary '```x'"
	achados := []veredito.Achado{lat, cerca}
	for i := 0; i < 13; i++ {
		achados = append(achados, achado(veredito.Diferenca, veredito.Branch, "GET /muitos"))
	}
	s := laudo(t, resultadoTeste(20, achados...))
	for _, esperado := range []string{`| GET /a\|b |`, "````sh\ncurl --data-binary '```x'\n````", "e mais 3 achados neste endpoint"} {
		if !strings.Contains(s, esperado) {
			t.Errorf("laudo sem %q:\n%s", esperado, s)
		}
	}
	if n := strings.Count(s, "status 0 → 0"); n != 11 {
		t.Errorf("itens de diferença no laudo: %d, esperado 11 (10 de /muitos + 1 de /c)", n)
	}
}
