package exploracao

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"
)

// intervaloVolateis é quanto a segunda chamada espera depois da primeira:
// mais de um segundo, para um horário com resolução de segundos mudar.
const intervaloVolateis = 1100 * time.Millisecond

// maxVolateis: acima disso, o corpo inteiro é tratado como volátil.
const maxVolateis = 50

// marcarVolateis chama cada rota de novo na mesma versão e marca na troca os
// campos que mudaram sozinhos (horário, contador, ordem): o replay não os
// compara, senão toda exploração terminaria em "revisar".
func (e *explorador) marcarVolateis(ctx context.Context) {
	for i := range e.trocas {
		t := &e.trocas[i]
		if espera := time.Until(t.Horario.Add(intervaloVolateis)); espera > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(espera):
			}
		}
		de, ok := e.buscar(ctx, item{caminho: t.Caminho, query: t.Query})
		if !ok || t.CorpoRespostaTruncado || de.CorpoRespostaTruncado {
			continue
		}
		t.CamposVolateis = volateisEntre(t.CorpoResposta, de.CorpoResposta)
		if len(t.CamposVolateis) > 0 {
			e.resumo.ComVolateis++
		}
	}
}

// volateisEntre devolve os caminhos JSON que diferem entre as duas respostas
// ("$" se não forem JSON e diferirem, ou se diferirem demais).
func volateisEntre(a, b []byte) []string {
	if bytes.Equal(a, b) {
		return nil
	}
	va, oka := decodificarJSON(a)
	vb, okb := decodificarJSON(b)
	if !oka || !okb {
		return []string{"$"}
	}
	var out []string
	diferencas("$", va, vb, &out)
	if len(out) == 0 || len(out) > maxVolateis {
		return []string{"$"}
	}
	return out
}

// diferencas usa a mesma notação de caminho do comparador do replay:
// "$.chave", "$.lista[0]".
func diferencas(caminho string, a, b any, out *[]string) {
	if len(*out) > maxVolateis {
		return
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		*out = append(*out, caminho)
		return
	}
	switch va := a.(type) {
	case map[string]any:
		vb := b.(map[string]any)
		chaves := map[string]bool{}
		for k := range va {
			chaves[k] = true
		}
		for k := range vb {
			chaves[k] = true
		}
		ks := make([]string, 0, len(chaves))
		for k := range chaves {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			x, okx := va[k]
			y, oky := vb[k]
			if !okx || !oky {
				*out = append(*out, caminho+"."+k)
				continue
			}
			diferencas(caminho+"."+k, x, y, out)
		}
	case []any:
		vb := b.([]any)
		if len(va) != len(vb) {
			*out = append(*out, caminho)
			return
		}
		for i := range va {
			diferencas(fmt.Sprintf("%s[%d]", caminho, i), va[i], vb[i], out)
		}
	default:
		if !reflect.DeepEqual(a, b) {
			*out = append(*out, caminho)
		}
	}
}
