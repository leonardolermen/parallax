package platform

import "testing"

// A latência de uma troca no loopback costuma ficar abaixo de 1 ms. O relógio
// usado para medi-la precisa enxergar intervalos de poucos microssegundos; no
// Windows o relógio monotônico do Go anda em passos de ~0,5 ms e mediria 0.
func TestRelogioMedeIntervalosCurtos(t *testing.T) {
	zeros := 0
	for i := 0; i < 200; i++ {
		inicio := Instante()
		soma := 0
		for j := 0; j < 20000; j++ {
			soma += j
		}
		_ = soma
		if DecorridoMs(inicio) <= 0 {
			zeros++
		}
	}
	if zeros > 0 {
		t.Fatalf("%d de 200 medições de alguns microssegundos deram 0 ms", zeros)
	}
}
