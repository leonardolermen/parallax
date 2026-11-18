package veredito

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func vereditoDeExemplo() Veredito {
	return Veredito{
		Versao:    Versao,
		ID:        "01JDB7Q8Z3X4M5N6P7R8S9T0VW",
		Resultado: Regressao,
		Resumo:    "1 regressão da branch.",
		Estatisticas: Estatisticas{
			Total:      10,
			Identicas:  8,
			Diferentes: 2,
			Latencia: Latencias{
				Base:      Percentis{P50: 10.5, P95: 20.25},
				Candidata: Percentis{P50: 11, P95: 30.5},
				Delta:     Percentis{P50: 0.5, P95: 10.25},
			},
		},
		Achados: []Achado{
			{
				Tipo:       ContratoQuebrado,
				Origem:     Branch,
				Endpoint:   "GET /orders/{id}",
				Reproducao: "curl -sS -X GET 'http://localhost:8080/orders/1'",
				Base:       Lado{Status: 200, Corpo: `{"a":1}`, LatenciaMs: 1.5, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"},
				Candidata:  Lado{Status: 200, Corpo: `{"a":"1"}`, LatenciaMs: 2.5, TraceID: "0af7651916cd43dd8448eb211c80319c"},
				Diff: []Mudanca{
					{Caminho: "$.a", Antes: json.RawMessage(`1`), Depois: json.RawMessage(`"1"`)},
				},
			},
			{
				Tipo:       ErroNovo,
				Origem:     PreExistente,
				Endpoint:   "GET /x",
				Reproducao: "curl -sS 'http://localhost:8080/x'",
				Base:       Lado{Status: 500, Corpo: "erro", LatenciaMs: 1, TraceID: "11111111111111111111111111111111"},
				Candidata:  Lado{Status: 0, LatenciaMs: 5000, TraceID: "22222222222222222222222222222222", Erro: "timeout"},
				Diff: []Mudanca{
					{Caminho: "$.b", Antes: json.RawMessage(`{"c":[1,2]}`)},
				},
				Local: "main.go:42",
			},
		},
	}
}

func TestIdaEVolta(t *testing.T) {
	original := vereditoDeExemplo()
	dados, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var relido Veredito
	if err := json.Unmarshal(dados, &relido); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, relido) {
		t.Errorf("ida e volta diferente:\noriginal: %+v\nrelido:   %+v", original, relido)
	}
}

func TestAchadosNuncaNull(t *testing.T) {
	dados, err := json.Marshal(Veredito{Versao: Versao, Resultado: Aprovado})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(dados, []byte(`"achados":[]`)) {
		t.Errorf("esperado \"achados\":[] em %s", dados)
	}
	// Também via ponteiro.
	dados, err = json.Marshal(&Veredito{Versao: Versao, Resultado: Aprovado})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(dados, []byte(`"achados":[]`)) {
		t.Errorf("esperado \"achados\":[] em %s (ponteiro)", dados)
	}
}

func carregarExemplo(t *testing.T) Veredito {
	t.Helper()
	f, err := os.Open("testdata/exemplo.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var v Veredito
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("exemplo.json não carrega no tipo: %v", err)
	}
	return v
}

func TestExemploCarregaEValida(t *testing.T) {
	v := carregarExemplo(t)
	if err := v.Validar(); err != nil {
		t.Fatalf("Validar: %v", err)
	}
	if v.Versao != Versao {
		t.Errorf("versao = %d, esperado %d", v.Versao, Versao)
	}
	if len(v.ID) != 26 {
		t.Errorf("id com %d caracteres, esperado 26", len(v.ID))
	}
	if len(v.Achados) < 4 {
		t.Fatalf("esperado ao menos 4 achados, veio %d", len(v.Achados))
	}
	tipos := map[Tipo]bool{}
	origens := map[Origem]bool{}
	for _, a := range v.Achados {
		tipos[a.Tipo] = true
		origens[a.Origem] = true
	}
	for _, tp := range []Tipo{ErroNovo, ContratoQuebrado, Latencia, Diferenca} {
		if !tipos[tp] {
			t.Errorf("exemplo sem achado do tipo %q", tp)
		}
	}
	for _, o := range []Origem{Branch, PreExistente} {
		if !origens[o] {
			t.Errorf("exemplo sem achado de origem %q", o)
		}
	}
}

// extrairExemploDoDoc devolve o primeiro bloco ```json depois do título
// "## Exemplo completo" em docs/contrato.md.
func extrairExemploDoDoc(t *testing.T) []byte {
	t.Helper()
	dados, err := os.ReadFile("../../docs/contrato.md")
	if err != nil {
		t.Fatal(err)
	}
	texto := strings.ReplaceAll(string(dados), "\r\n", "\n")
	i := strings.Index(texto, "## Exemplo completo")
	if i < 0 {
		t.Fatal(`docs/contrato.md sem o título "## Exemplo completo"`)
	}
	texto = texto[i:]
	const abre = "```json\n"
	j := strings.Index(texto, abre)
	if j < 0 {
		t.Fatal("docs/contrato.md sem bloco ```json depois de \"## Exemplo completo\"")
	}
	texto = texto[j+len(abre):]
	k := strings.Index(texto, "\n```")
	if k < 0 {
		t.Fatal("bloco ```json sem fechamento em docs/contrato.md")
	}
	return []byte(texto[:k])
}

func TestDocIgualAoExemplo(t *testing.T) {
	doDoc := extrairExemploDoDoc(t)
	doArquivo, err := os.ReadFile("testdata/exemplo.json")
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(doDoc, &a); err != nil {
		t.Fatalf("JSON do docs/contrato.md inválido: %v", err)
	}
	if err := json.Unmarshal(doArquivo, &b); err != nil {
		t.Fatalf("testdata/exemplo.json inválido: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("o exemplo de docs/contrato.md difere de pkg/veredito/testdata/exemplo.json")
	}
}

func TestValidar(t *testing.T) {
	valido := func() Veredito { return vereditoDeExemplo() }
	casos := []struct {
		nome   string
		mudar  func(v *Veredito)
		erro   bool
		trecho string
	}{
		{"válido", func(v *Veredito) {}, false, ""},
		{"versão 2", func(v *Veredito) { v.Versao = 2 }, true, "versao"},
		{"resultado desconhecido", func(v *Veredito) { v.Resultado = "x" }, true, "resultado"},
		{"tipo desconhecido", func(v *Veredito) { v.Achados[0].Tipo = "x" }, true, "achados[0].tipo"},
		{"origem desconhecida", func(v *Veredito) { v.Achados[1].Origem = "x" }, true, "achados[1].origem"},
		{"resultado não bate", func(v *Veredito) {
			v.Resultado = Aprovado
			v.Achados = []Achado{{Tipo: ErroNovo, Origem: Branch}}
		}, true, "resultado"},
		{"aprovado sem achados", func(v *Veredito) {
			v.Resultado = Aprovado
			v.Achados = nil
		}, false, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			v := valido()
			c.mudar(&v)
			err := v.Validar()
			if c.erro && err == nil {
				t.Fatal("esperado erro, veio nil")
			}
			if !c.erro && err != nil {
				t.Fatalf("esperado nil, veio %v", err)
			}
			if c.erro && !strings.Contains(err.Error(), c.trecho) {
				t.Errorf("erro %q não cita %q", err, c.trecho)
			}
		})
	}
	if err := carregarExemplo(t).Validar(); err != nil {
		t.Errorf("exemplo.json deveria ser válido: %v", err)
	}
}
