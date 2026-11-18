package veredito

import (
	"errors"
	"fmt"
)

// CalcularResultado aplica a regra do resultado aos achados:
//
//   - regressao: algum achado erro_novo, contrato_quebrado ou latencia com
//     origem branch;
//   - revisar: há achados, mas só de diferença ou pré-existentes;
//   - aprovado: nenhum achado.
func CalcularResultado(achados []Achado) Resultado {
	if len(achados) == 0 {
		return Aprovado
	}
	for _, a := range achados {
		if a.Origem != Branch {
			continue
		}
		switch a.Tipo {
		case ErroNovo, ContratoQuebrado, Latencia:
			return Regressao
		}
	}
	return Revisar
}

// CodigoSaida devolve o código de saída da CLI para o resultado:
// aprovado 0, regressao 1, revisar 2 e qualquer outro valor SaidaErro (4).
func CodigoSaida(r Resultado) int {
	switch r {
	case Aprovado:
		return SaidaAprovado
	case Regressao:
		return SaidaRegressao
	case Revisar:
		return SaidaRevisar
	}
	return SaidaErro
}

func resultadoValido(r Resultado) bool {
	return r == Aprovado || r == Regressao || r == Revisar
}

func tipoValido(t Tipo) bool {
	return t == ErroNovo || t == ContratoQuebrado || t == Latencia || t == Diferenca
}

func origemValida(o Origem) bool {
	return o == Branch || o == PreExistente
}

// Validar confere a versão, os valores permitidos de resultado, tipo e origem,
// e se o resultado bate com CalcularResultado(achados).
func (v Veredito) Validar() error {
	var erros []error
	if v.Versao != Versao {
		erros = append(erros, fmt.Errorf("versao: esperado %d, veio %d", Versao, v.Versao))
	}
	if !resultadoValido(v.Resultado) {
		erros = append(erros, fmt.Errorf("resultado: valor desconhecido %q", v.Resultado))
	}
	for i, a := range v.Achados {
		if !tipoValido(a.Tipo) {
			erros = append(erros, fmt.Errorf("achados[%d].tipo: valor desconhecido %q", i, a.Tipo))
		}
		if !origemValida(a.Origem) {
			erros = append(erros, fmt.Errorf("achados[%d].origem: valor desconhecido %q", i, a.Origem))
		}
	}
	if resultadoValido(v.Resultado) {
		if esperado := CalcularResultado(v.Achados); esperado != v.Resultado {
			erros = append(erros, fmt.Errorf("resultado: %q não bate com os achados (esperado %q)", v.Resultado, esperado))
		}
	}
	return errors.Join(erros...)
}
