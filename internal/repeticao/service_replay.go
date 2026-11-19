package repeticao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/leonardolermen/parallax/internal/platform"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

type Estatisticas struct {
	Total      int                `json:"total"`
	Identicas  int                `json:"identicas"`
	Diferentes int                `json:"diferentes"`
	Puladas    int                `json:"puladas"`
	Latencia   veredito.Latencias `json:"latencia"`
}
type Resultado struct {
	ID           string            `json:"id"`
	Estatisticas Estatisticas      `json:"estatisticas"`
	Achados      []veredito.Achado `json:"achados"`
}

func (r Resultado) MarshalJSON() ([]byte, error) {
	type plain Resultado
	p := plain(r)
	if p.Achados == nil {
		p.Achados = []veredito.Achado{}
	}
	return json.Marshal(p)
}

// Repetir valida toda a entrada antes de enviar qualquer requisicao.
func Repetir(ctx context.Context, c Config, r io.Reader) (Resultado, error) {
	out := Resultado{Achados: []veredito.Achado{}}
	if err := c.Validar(); err != nil {
		return out, err
	}
	ts, err := carregar(ctx, r)
	if err != nil {
		return out, err
	}
	ps, err := executar(ctx, c, ts)
	if err != nil {
		return out, fmt.Errorf("motor: repetir: %w", err)
	}
	out.ID = platform.NovoID()
	type grupo struct {
		base, candidata []float64
		exemplo         par
	}
	grupos := map[string]*grupo{}
	var bs, cs []float64
	for _, p := range ps {
		if p.pulada {
			out.Estatisticas.Puladas++
			continue
		}
		out.Estatisticas.Total++
		achados := comparar(p.troca, c, p.base, p.candidata)
		if len(achados) == 0 {
			out.Estatisticas.Identicas++
		} else {
			out.Estatisticas.Diferentes++
		}
		out.Achados = append(out.Achados, achados...)
		if p.base.Incompleta || p.candidata.Incompleta || p.base.Erro != "" || p.candidata.Erro != "" {
			continue
		}
		bs = append(bs, p.base.LatenciaMs)
		cs = append(cs, p.candidata.LatenciaMs)
		ep := Endpoint(p.troca.Metodo, p.troca.Caminho)
		g := grupos[ep]
		if g == nil {
			g = &grupo{exemplo: p}
			grupos[ep] = g
		}
		g.base = append(g.base, p.base.LatenciaMs)
		g.candidata = append(g.candidata, p.candidata.LatenciaMs)
	}
	out.Estatisticas.Latencia = latencias(bs, cs)
	keys := make([]string, 0, len(grupos))
	for k := range grupos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, ep := range keys {
		g := grupos[ep]
		l := latencias(g.base, g.candidata)
		if ok, o := Regressao(l.Base.P95, l.Candidata.P95, c); ok {
			b, a := g.exemplo.base.Lado, g.exemplo.candidata.Lado
			b.LatenciaMs = l.Base.P95
			a.LatenciaMs = l.Candidata.P95
			out.Achados = append(out.Achados, veredito.Achado{Tipo: veredito.Latencia, Origem: o, Endpoint: ep, Reproducao: curl(g.exemplo.troca, c), Base: b, Candidata: a})
		}
	}
	return out, nil
}
