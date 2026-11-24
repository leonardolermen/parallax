package repeticao

import (
	"net/http"
	"testing"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func respostaJSON(corpo string) resposta {
	r := resposta{Corpo: []byte(corpo), Cabecalhos: http.Header{"Content-Type": {"application/json"}}}
	r.Status = 200
	return r
}

func TestCompararIgnoraCamposVolateis(t *testing.T) {
	c := ConfigPadrao()
	c.Base, c.Candidata = "http://b", "http://a"
	tr := captura.Troca{Metodo: "GET", Caminho: "/x", CamposVolateis: []string{"$.em", "$.itens"}}
	b := respostaJSON(`{"id":1,"em":"10:00:01","itens":[{"visto":1}]}`)
	a := respostaJSON(`{"id":1,"em":"10:00:02","itens":[{"visto":2},{"visto":3}]}`)
	if achados := comparar(tr, c, b, a); len(achados) != 0 {
		t.Fatalf("achou diferença em campo volátil: %+v", achados)
	}

	// fora dos voláteis continua valendo, e o contrato quebrado é recalculado sem eles
	a = respostaJSON(`{"em":5,"itens":[]}`)
	achados := comparar(tr, c, b, a)
	if len(achados) != 1 || achados[0].Tipo != veredito.ContratoQuebrado || len(achados[0].Diff) != 1 || achados[0].Diff[0].Caminho != "$.id" {
		t.Fatalf("achados: %+v", achados)
	}
	a = respostaJSON(`{"id":2,"em":5,"itens":[]}`)
	achados = comparar(tr, c, b, a)
	if len(achados) != 1 || achados[0].Tipo != veredito.Diferenca {
		t.Fatalf("a troca de tipo de um campo volátil não quebra o contrato: %+v", achados)
	}
}

func TestCompararCorpoInteiroVolatil(t *testing.T) {
	c := ConfigPadrao()
	c.Base, c.Candidata = "http://b", "http://a"
	tr := captura.Troca{Metodo: "GET", Caminho: "/", CamposVolateis: []string{"$"}}
	b, a := respostaJSON("<p>10:00:01</p>"), respostaJSON("<p>10:00:02</p>")
	if achados := comparar(tr, c, b, a); len(achados) != 0 {
		t.Fatalf("achados: %+v", achados)
	}
	a.Status = 500
	if achados := comparar(tr, c, b, a); len(achados) != 1 || achados[0].Tipo != veredito.ErroNovo {
		t.Fatalf("o status continua valendo: %+v", achados)
	}
}
