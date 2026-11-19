package repeticao

import (
	"math"
	"sort"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// Percentil aplica nearest-rank sem modificar as amostras.
func Percentil(amostras []float64, p float64) float64 {
	if len(amostras) == 0 {
		return 0
	}
	s := append([]float64(nil), amostras...)
	sort.Float64s(s)
	i := int(math.Ceil(p/100*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

// Regressao informa se o p95 viola o limite relativo ou absoluto.
func Regressao(base, candidata float64, c Config) (bool, veredito.Origem) {
	if c.LimiteAbsolutoMs > 0 && candidata > c.LimiteAbsolutoMs {
		if base > c.LimiteAbsolutoMs {
			return true, veredito.PreExistente
		}
		return true, veredito.Branch
	}
	if candidata > base*(1+c.PercentualLatencia/100) && candidata-base >= c.MinimoLatenciaMs {
		return true, veredito.Branch
	}
	return false, veredito.Branch
}
func percentis(s []float64) veredito.Percentis {
	return veredito.Percentis{P50: Percentil(s, 50), P95: Percentil(s, 95)}
}
func latencias(b, c []float64) veredito.Latencias {
	pb, pc := percentis(b), percentis(c)
	return veredito.Latencias{Base: pb, Candidata: pc, Delta: veredito.Percentis{P50: pc.P50 - pb.P50, P95: pc.P95 - pb.P95}}
}
