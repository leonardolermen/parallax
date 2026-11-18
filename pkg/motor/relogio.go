//go:build !windows

package motor

import "time"

// marca é um instante do relógio usado para medir latência.
type marca = time.Time

// instante lê o relógio monotônico do Go, que fora do Windows tem resolução
// de nanossegundos.
func instante() marca { return time.Now() }

// decorridoMs devolve os milissegundos desde inicio, com casas decimais.
func decorridoMs(inicio marca) float64 {
	return float64(time.Since(inicio)) / float64(time.Millisecond)
}
