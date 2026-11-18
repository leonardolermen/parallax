package veredito

import "testing"

func TestCalcularResultado(t *testing.T) {
	a := func(tp Tipo, o Origem) Achado { return Achado{Tipo: tp, Origem: o} }
	casos := []struct {
		nome     string
		achados  []Achado
		esperado Resultado
	}{
		{"nil", nil, Aprovado},
		{"vazio", []Achado{}, Aprovado},
		{"erro novo na branch", []Achado{a(ErroNovo, Branch)}, Regressao},
		{"contrato quebrado na branch", []Achado{a(ContratoQuebrado, Branch)}, Regressao},
		{"latencia na branch", []Achado{a(Latencia, Branch)}, Regressao},
		{"diferenca na branch", []Achado{a(Diferenca, Branch)}, Revisar},
		{"erro novo pre-existente", []Achado{a(ErroNovo, PreExistente)}, Revisar},
		{"latencia pre-existente", []Achado{a(Latencia, PreExistente)}, Revisar},
		{"diferenca e pre-existente", []Achado{a(Diferenca, Branch), a(ErroNovo, PreExistente)}, Revisar},
		{"diferenca e contrato quebrado", []Achado{a(Diferenca, Branch), a(ContratoQuebrado, Branch)}, Regressao},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := CalcularResultado(c.achados); got != c.esperado {
				t.Errorf("CalcularResultado = %q, esperado %q", got, c.esperado)
			}
		})
	}
}

func TestCodigoSaida(t *testing.T) {
	casos := []struct {
		r        Resultado
		esperado int
	}{
		{Aprovado, 0},
		{Regressao, 1},
		{Revisar, 2},
		{"", 4},
		{"outro", 4},
	}
	for _, c := range casos {
		if got := CodigoSaida(c.r); got != c.esperado {
			t.Errorf("CodigoSaida(%q) = %d, esperado %d", c.r, got, c.esperado)
		}
	}
	constantes := []struct {
		nome  string
		valor int
		quer  int
	}{
		{"SaidaAprovado", SaidaAprovado, 0},
		{"SaidaRegressao", SaidaRegressao, 1},
		{"SaidaRevisar", SaidaRevisar, 2},
		{"SaidaNaoImplementado", SaidaNaoImplementado, 3},
		{"SaidaErro", SaidaErro, 4},
	}
	for _, c := range constantes {
		if c.valor != c.quer {
			t.Errorf("%s = %d, esperado %d", c.nome, c.valor, c.quer)
		}
	}
}
