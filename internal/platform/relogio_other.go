//go:build !windows

package platform

import "time"

// Marca é um instante do relógio usado para medir latência.
type Marca = time.Time

// Instante lê o relógio monotônico do Go, que fora do Windows tem resolução
// de nanossegundos.
func Instante() Marca { return time.Now() }

// DecorridoMs devolve os milissegundos desde inicio, com casas decimais.
func DecorridoMs(inicio Marca) float64 {
	return float64(time.Since(inicio)) / float64(time.Millisecond)
}
