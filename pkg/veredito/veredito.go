// Package veredito define o contrato do arquivo veredito.json produzido pelo
// Parallax: os tipos, a regra que calcula o resultado e o código de saída da
// CLI.
//
// Este pacote é importado pelo Solaris e por isso só depende da biblioteca
// padrão. Nada aqui pode importar pacotes internos do Parallax.
//
// A documentação completa do formato está em docs/contrato.md.
package veredito

import "encoding/json"

// Versao é a versão do formato do veredito.json. Ela muda quando o contrato
// muda de forma incompatível.
const Versao = 1

// Resultado é a conclusão geral de uma execução.
type Resultado string

// Valores possíveis de Resultado.
const (
	// Aprovado: nenhum achado.
	Aprovado Resultado = "aprovado"
	// Regressao: algum achado erro_novo, contrato_quebrado ou latencia com
	// origem branch.
	Regressao Resultado = "regressao"
	// Revisar: só há achados de diferença ou pré-existentes.
	Revisar Resultado = "revisar"
)

// Tipo classifica um achado.
type Tipo string

// Valores possíveis de Tipo.
const (
	// ErroNovo: 5xx, timeout ou conexão recusada na candidata.
	ErroNovo Tipo = "erro_novo"
	// ContratoQuebrado: campo removido ou tipo trocado na resposta.
	ContratoQuebrado Tipo = "contrato_quebrado"
	// Latencia: p95 do endpoint acima do limite.
	Latencia Tipo = "latencia"
	// Diferenca: a resposta mudou, mas não dá para afirmar que é bug.
	Diferenca Tipo = "diferenca"
)

// Origem diz se o problema veio da branch ou já existia na base.
type Origem string

// Valores possíveis de Origem.
const (
	// Branch: só a candidata falha.
	Branch Origem = "branch"
	// PreExistente: a base também falha.
	PreExistente Origem = "pre_existente"
)

// Códigos de saída da CLI.
const (
	SaidaAprovado        = 0 // resultado aprovado
	SaidaRegressao       = 1 // resultado regressao
	SaidaRevisar         = 2 // resultado revisar
	SaidaNaoImplementado = 3 // subcomando ainda não implementado
	SaidaErro            = 4 // uso errado ou falha de execução
)

// Veredito é o conteúdo do arquivo veredito.json.
type Veredito struct {
	// Versao do formato; sempre igual à constante Versao.
	Versao int `json:"versao"`
	// ID da execução (ULID, 26 caracteres).
	ID string `json:"id"`
	// Resultado geral, calculado por CalcularResultado a partir dos achados.
	Resultado Resultado `json:"resultado"`
	// Resumo em uma frase, para humanos.
	Resumo string `json:"resumo"`
	// Estatisticas da repetição.
	Estatisticas Estatisticas `json:"estatisticas"`
	// Achados encontrados. Nunca é null no JSON: vira [] quando vazio.
	Achados []Achado `json:"achados"`
}

// MarshalJSON serializa o veredito garantindo "achados": [] quando não há
// achados.
func (v Veredito) MarshalJSON() ([]byte, error) {
	type semMetodos Veredito
	c := semMetodos(v)
	if c.Achados == nil {
		c.Achados = []Achado{}
	}
	return json.Marshal(c)
}

// Estatisticas resume a repetição do tráfego.
type Estatisticas struct {
	// Total de trocas repetidas.
	Total int `json:"total"`
	// Identicas: trocas com resposta igual nos dois lados.
	Identicas int `json:"identicas"`
	// Diferentes: trocas com alguma diferença entre os lados.
	Diferentes int `json:"diferentes"`
	// Latencia de cada lado e o delta.
	Latencia Latencias `json:"latencia"`
}

// Latencias traz os percentis de cada lado e a diferença entre eles.
type Latencias struct {
	// Base: percentis da versão base.
	Base Percentis `json:"base"`
	// Candidata: percentis da versão candidata.
	Candidata Percentis `json:"candidata"`
	// Delta: candidata menos base (positivo = candidata mais lenta).
	Delta Percentis `json:"delta"`
}

// Percentis de latência, em milissegundos com casas decimais.
type Percentis struct {
	// P50 é a mediana, em ms.
	P50 float64 `json:"p50_ms"`
	// P95 é o percentil 95, em ms.
	P95 float64 `json:"p95_ms"`
}

// Achado descreve um problema ou uma diferença encontrada.
type Achado struct {
	// Tipo do achado.
	Tipo Tipo `json:"tipo"`
	// Origem: branch (só a candidata) ou pre_existente (a base também).
	Origem Origem `json:"origem"`
	// Endpoint: método e rota normalizada, ex.: "GET /orders/{id}".
	Endpoint string `json:"endpoint"`
	// Reproducao: um curl pronto para rodar.
	Reproducao string `json:"reproducao"`
	// Base: o que a versão base respondeu.
	Base Lado `json:"base"`
	// Candidata: o que a versão candidata respondeu.
	Candidata Lado `json:"candidata"`
	// Diff: caminhos JSON que mudaram, com valor antes e depois.
	Diff []Mudanca `json:"diff,omitempty"`
	// Local: arquivo:linha, quando a resposta trouxe stack trace.
	Local string `json:"local,omitempty"`
}

// Lado é a resposta de uma das versões (base ou candidata).
type Lado struct {
	// Status HTTP; 0 quando não houve resposta (ver Erro).
	Status int `json:"status"`
	// Corpo resumido da resposta.
	Corpo string `json:"corpo"`
	// LatenciaMs da requisição, em ms com casas decimais.
	LatenciaMs float64 `json:"latencia_ms"`
	// TraceID do traceparent enviado pelo Parallax (32 hex).
	TraceID string `json:"trace_id"`
	// Erro quando não houve resposta: "timeout", "conexao_recusada"...
	Erro string `json:"erro,omitempty"`
}

// Mudanca é um caminho JSON que mudou entre base e candidata.
type Mudanca struct {
	// Caminho JSON, ex.: "$.items[0].price".
	Caminho string `json:"caminho"`
	// Antes: valor na base. Ausente = o campo não existia na base.
	Antes json.RawMessage `json:"antes,omitempty"`
	// Depois: valor na candidata. Ausente = o campo foi removido.
	Depois json.RawMessage `json:"depois,omitempty"`
}
