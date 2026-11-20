package julgamento

import (
	"errors"
	"strings"
	"testing"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

const idTeste = "01JD0000000000000000000000"

func resultadoTeste(total int, achados ...veredito.Achado) repeticao.Resultado {
	r := repeticao.Resultado{ID: idTeste, Achados: achados}
	r.Estatisticas.Total = total
	r.Estatisticas.Diferentes = len(achados)
	r.Estatisticas.Identicas = total - len(achados)
	r.Estatisticas.Puladas = 3
	r.Estatisticas.Latencia.Base.P95 = 10
	return r
}

func achado(tp veredito.Tipo, o veredito.Origem, ep string) veredito.Achado {
	return veredito.Achado{Tipo: tp, Origem: o, Endpoint: ep}
}

func TestJulgar(t *testing.T) {
	casos := []struct {
		nome           string
		r              repeticao.Resultado
		resultado      veredito.Resultado
		resumo, motivo string
	}{
		{"aprovado", resultadoTeste(5), veredito.Aprovado,
			"aprovado: 5 trocas comparadas sem achados.", "nenhum achado"},
		{"regressao", resultadoTeste(5,
			achado(veredito.Diferenca, veredito.Branch, "GET /a"),
			achado(veredito.ErroNovo, veredito.Branch, "GET /b"),
			achado(veredito.Latencia, veredito.Branch, "GET /c"),
			achado(veredito.ContratoQuebrado, veredito.Branch, "GET /d"),
			achado(veredito.ErroNovo, veredito.Branch, "GET /e"),
			achado(veredito.ErroNovo, veredito.PreExistente, "GET /f")),
			veredito.Regressao,
			"regressao: 4 regressões da branch em 5 trocas: erro_novo em GET /b, latencia em GET /c, contrato_quebrado em GET /d e mais 1.",
			"achado erro_novo com origem branch em GET /b"},
		{"revisar", resultadoTeste(5,
			achado(veredito.Diferenca, veredito.Branch, "GET /a"),
			achado(veredito.ErroNovo, veredito.PreExistente, "GET /b")),
			veredito.Revisar,
			"revisar: 2 achados para revisar em 5 trocas (0 comparações incompletas).",
			"só há diferenças ou achados pré-existentes"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			v, err := Julgar(c.r)
			if err != nil {
				t.Fatal(err)
			}
			if v.Resultado != c.resultado || v.Resumo != c.resumo {
				t.Fatalf("resultado %q, resumo %q", v.Resultado, v.Resumo)
			}
			if err := v.Validar(); err != nil {
				t.Fatal(err)
			}
			if v.ID != idTeste || v.Versao != veredito.Versao || v.Estatisticas.Total != 5 || v.Estatisticas.Latencia.Base.P95 != 10 {
				t.Fatalf("veredito: %+v", v)
			}
			if m := Motivo(v); !strings.Contains(m, c.motivo) {
				t.Fatalf("motivo %q sem %q", m, c.motivo)
			}
		})
	}
}

func TestJulgarIncompletaDaRevisar(t *testing.T) {
	a := achado(veredito.Diferenca, veredito.Branch, "GET /grande")
	a.Candidata.Corpo = "xxxx" + repeticao.MarcadorIncompleta
	r := resultadoTeste(2, a)
	r.Estatisticas.Incompletas = 1
	v, err := Julgar(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.Resultado != veredito.Revisar || !strings.Contains(v.Resumo, "1 comparações incompletas") {
		t.Fatalf("%q %q", v.Resultado, v.Resumo)
	}
}

func TestJulgarSemEvidencia(t *testing.T) {
	if _, err := Julgar(resultadoTeste(0)); !errors.Is(err, ErrSemEvidencia) {
		t.Fatalf("erro %v", err)
	}
}
