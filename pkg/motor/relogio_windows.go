//go:build windows

package motor

import (
	"syscall"
	"time"
	"unsafe"
)

// No Windows o relógio monotônico do Go (time.Now/time.Since) anda em passos
// de ~0,5 ms ou mais, e uma troca no loopback mediria 0 ms. Aqui a latência
// usa o QueryPerformanceCounter, com resolução de ~100 ns.

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	procContador = kernel32.NewProc("QueryPerformanceCounter")
	procFreq     = kernel32.NewProc("QueryPerformanceFrequency")
	frequencia   = lerFrequencia() // tiques por segundo; 0 se indisponível
)

// marca é um instante do relógio usado para medir latência.
type marca struct {
	tiques int64     // QueryPerformanceCounter; válido só se frequencia > 0
	tempo  time.Time // reserva, caso o contador não esteja disponível
}

func lerFrequencia() int64 {
	var f int64
	if procFreq.Find() != nil || procContador.Find() != nil {
		return 0
	}
	if r, _, _ := procFreq.Call(uintptr(unsafe.Pointer(&f))); r == 0 {
		return 0
	}
	return f
}

func lerContador() int64 {
	var c int64
	procContador.Call(uintptr(unsafe.Pointer(&c)))
	return c
}

// instante lê o contador de alta resolução.
func instante() marca {
	if frequencia > 0 {
		return marca{tiques: lerContador()}
	}
	return marca{tempo: time.Now()}
}

// decorridoMs devolve os milissegundos desde inicio, com casas decimais.
func decorridoMs(inicio marca) float64 {
	if frequencia > 0 {
		return float64(lerContador()-inicio.tiques) * 1000 / float64(frequencia)
	}
	return float64(time.Since(inicio.tempo)) / float64(time.Millisecond)
}
