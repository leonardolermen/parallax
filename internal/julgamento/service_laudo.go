package julgamento

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

// maxDiffLaudo limita quantas mudanças de cada achado aparecem no laudo.
const maxDiffLaudo = 10

// maxValorLaudo limita o tamanho de cada valor antes/depois no laudo.
const maxValorLaudo = 80

// EscreverLaudo escreve o laudo em Markdown: veredito, motivo, estatísticas,
// regressões de latência (p95 antes/depois), diferenças por endpoint e
// comparações incompletas. Seções sem itens mostram "Nenhuma.".
func EscreverLaudo(w io.Writer, e Execucao, v veredito.Veredito) error {
	b := bufio.NewWriter(w)
	p := func(format string, args ...any) { fmt.Fprintf(b, format, args...) }

	p("# Laudo Parallax %s\n\n", e.ID)
	p("- Data: %s\n- Captura: %s\n- Base: %s\n- Candidata: %s\n\n", e.Data.UTC().Format(time.RFC3339), e.Captura, e.Base, e.Candidata)

	p("## Veredito\n\n**%s**: %s\n\nMotivo: %s\n\n", strings.ToUpper(string(v.Resultado)), v.Resumo, Motivo(v))

	est, l := e.Estatisticas, v.Estatisticas.Latencia
	p("## Estatísticas\n\n| Trocas | Quantidade |\n|---|---:|\n")
	p("| Total | %d |\n| Idênticas | %d |\n| Diferentes | %d |\n| Puladas | %d |\n| Incompletas | %d |\n\n",
		v.Estatisticas.Total, v.Estatisticas.Identicas, v.Estatisticas.Diferentes, est.Puladas, est.Incompletas)
	p("| Latência (ms) | p50 | p95 |\n|---|---:|---:|\n")
	p("| Base | %.1f | %.1f |\n| Candidata | %.1f | %.1f |\n| Delta | %+.1f | %+.1f |\n\n",
		l.Base.P50, l.Base.P95, l.Candidata.P50, l.Candidata.P95, l.Delta.P50, l.Delta.P95)

	var latencias, diferencas, incompletas []veredito.Achado
	for _, a := range v.Achados {
		if a.Tipo == veredito.Latencia {
			latencias = append(latencias, a)
			continue
		}
		diferencas = append(diferencas, a)
		if a.Tipo == veredito.Diferenca && (incompleta(a.Base) || incompleta(a.Candidata)) {
			incompletas = append(incompletas, a)
		}
	}

	p("## Regressões de latência\n\n")
	if len(latencias) == 0 {
		p("Nenhuma.\n\n")
	} else {
		p("| Endpoint | p95 antes (ms) | p95 depois (ms) | Variação | Origem |\n|---|---:|---:|---:|---|\n")
		for _, a := range latencias {
			p("| %s | %.1f | %.1f | %s | %s |\n", a.Endpoint, a.Base.LatenciaMs, a.Candidata.LatenciaMs, variacao(a.Base.LatenciaMs, a.Candidata.LatenciaMs), a.Origem)
		}
		p("\n")
	}

	p("## Diferenças por endpoint\n\n")
	if len(diferencas) == 0 {
		p("Nenhuma.\n\n")
	}
	sort.SliceStable(diferencas, func(i, j int) bool { return diferencas[i].Endpoint < diferencas[j].Endpoint })
	for i, a := range diferencas {
		if i == 0 || diferencas[i-1].Endpoint != a.Endpoint {
			p("### %s\n\n", a.Endpoint)
		}
		p("- **%s** (%s): status %s → %s\n", a.Tipo, a.Origem, status(a.Base), status(a.Candidata))
		for j, m := range a.Diff {
			if j == maxDiffLaudo {
				p("  - e mais %d mudanças\n", len(a.Diff)-maxDiffLaudo)
				break
			}
			p("  - `%s`: %s → %s\n", m.Caminho, valor(m.Antes), valor(m.Depois))
		}
		if a.Local != "" {
			p("  - Local: %s\n", a.Local)
		}
		if a.Reproducao != "" {
			p("\n```sh\n%s\n```\n", a.Reproducao)
		}
		p("\n")
	}

	p("## Comparações incompletas\n\n")
	if len(incompletas) == 0 {
		p("Nenhuma.\n")
	}
	for _, a := range incompletas {
		var lados []string
		if incompleta(a.Base) {
			lados = append(lados, "base")
		}
		if incompleta(a.Candidata) {
			lados = append(lados, "candidata")
		}
		p("- %s: %s sem a resposta inteira (corpo acima de --max-body ou leitura interrompida)\n", a.Endpoint, strings.Join(lados, " e "))
	}
	return b.Flush()
}

func incompleta(l veredito.Lado) bool {
	return strings.HasSuffix(l.Corpo, repeticao.MarcadorIncompleta)
}

func status(l veredito.Lado) string {
	if l.Erro != "" {
		return l.Erro
	}
	return fmt.Sprint(l.Status)
}

func variacao(antes, depois float64) string {
	if antes == 0 {
		return "n/d"
	}
	return fmt.Sprintf("%+.1f%%", (depois-antes)/antes*100)
}

func valor(raw []byte) string {
	if len(raw) == 0 {
		return "(ausente)"
	}
	s := []rune(string(raw))
	if len(s) > maxValorLaudo {
		return string(s[:maxValorLaudo]) + "…"
	}
	return string(s)
}
