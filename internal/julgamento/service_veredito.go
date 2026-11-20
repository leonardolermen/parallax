// Package julgamento transforma o resultado de uma repetição no veredito do
// contrato (pkg/veredito), escreve o laudo legível por humanos e guarda cada
// execução no histórico local.
package julgamento

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

// ErrSemEvidencia: nenhuma troca foi comparada (tudo pulado), então não há
// como aprovar nem reprovar.
var ErrSemEvidencia = errors.New("nenhuma troca comparada")

// Execucao é o registro de uma execução do check no histórico
// (execucao.json).
type Execucao struct {
	ID           string                 `json:"id"`
	Data         time.Time              `json:"data"`
	Captura      string                 `json:"captura"`
	Base         string                 `json:"base"`
	Candidata    string                 `json:"candidata"`
	Resultado    veredito.Resultado     `json:"resultado"`
	Resumo       string                 `json:"resumo"`
	Estatisticas repeticao.Estatisticas `json:"estatisticas"`
}

// maxEndpointsResumo limita quantos endpoints aparecem no resumo da regressão.
const maxEndpointsResumo = 3

// Julgar monta o veredito a partir do resultado da repetição, usando
// veredito.CalcularResultado. Devolve ErrSemEvidencia quando nenhuma troca
// foi comparada.
func Julgar(r repeticao.Resultado) (veredito.Veredito, error) {
	e := r.Estatisticas
	if e.Total == 0 {
		return veredito.Veredito{}, ErrSemEvidencia
	}
	v := veredito.Veredito{
		Versao:    veredito.Versao,
		ID:        r.ID,
		Resultado: veredito.CalcularResultado(r.Achados),
		Estatisticas: veredito.Estatisticas{
			Total:      e.Total,
			Identicas:  e.Identicas,
			Diferentes: e.Diferentes,
			Latencia:   e.Latencia,
		},
		Achados: r.Achados,
	}
	v.Resumo = resumo(v, e.Incompletas)
	if err := v.Validar(); err != nil {
		return veredito.Veredito{}, fmt.Errorf("julgamento: veredito inválido: %w", err)
	}
	return v, nil
}

// regressivo diz se o achado sozinho reprova a candidata (mesma regra de
// veredito.CalcularResultado).
func regressivo(a veredito.Achado) bool {
	return veredito.CalcularResultado([]veredito.Achado{a}) == veredito.Regressao
}

func resumo(v veredito.Veredito, incompletas int) string {
	n := v.Estatisticas.Total
	switch v.Resultado {
	case veredito.Aprovado:
		return fmt.Sprintf("aprovado: %d trocas comparadas sem achados.", n)
	case veredito.Regressao:
		var itens []string
		vistos := map[string]bool{}
		k := 0
		for _, a := range v.Achados {
			if !regressivo(a) {
				continue
			}
			k++
			item := fmt.Sprintf("%s em %s", a.Tipo, a.Endpoint)
			if !vistos[item] {
				vistos[item] = true
				itens = append(itens, item)
			}
		}
		resto := ""
		if len(itens) > maxEndpointsResumo {
			resto = fmt.Sprintf(" e mais %d", len(itens)-maxEndpointsResumo)
			itens = itens[:maxEndpointsResumo]
		}
		return fmt.Sprintf("regressao: %d regressões da branch em %d trocas: %s%s.", k, n, strings.Join(itens, ", "), resto)
	default:
		return fmt.Sprintf("revisar: %d achados para revisar em %d trocas (%d comparações incompletas).", len(v.Achados), n, incompletas)
	}
}

// Motivo explica em uma frase a regra que decidiu o resultado.
func Motivo(v veredito.Veredito) string {
	switch v.Resultado {
	case veredito.Aprovado:
		return "nenhum achado: base e candidata responderam igual em todas as trocas comparadas."
	case veredito.Regressao:
		for _, a := range v.Achados {
			if regressivo(a) {
				return fmt.Sprintf("achado %s com origem branch em %s: erro novo, contrato quebrado ou regressão de latência introduzidos pela candidata reprovam.", a.Tipo, a.Endpoint)
			}
		}
	case veredito.Revisar:
		return "só há diferenças ou achados pré-existentes: nada prova regressão, mas é preciso revisar."
	}
	return string(v.Resultado)
}
