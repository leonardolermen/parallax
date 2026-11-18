// Package motor é o núcleo do Parallax como biblioteca: gravação de tráfego,
// e, nas próximas demandas, repetição, comparação e classificação.
//
// O motor não lê flags nem escreve no terminal: recebe configuração por
// structs e devolve valores e erros. A CLI (cmd/parallax) é uma camada fina
// por cima dele.
package motor

import (
	"net/http"
	"time"
)

// Troca é uma requisição HTTP e a resposta que o cliente recebeu, como fica
// gravada numa linha da captura JSONL.
//
// As tags json são estáveis: mudá-las quebra capturas já gravadas.
// Os corpos ([]byte) viram base64 padrão no JSON; corpo vazio é "" (o campo
// nunca é omitido). Os cabeçalhos sensíveis (Authorization, Cookie,
// Set-Cookie, X-Api-Key) ficam gravados com o valor "***".
type Troca struct {
	// Horario do início da requisição, em UTC, RFC3339Nano no JSON.
	Horario time.Time `json:"horario"`
	// Metodo HTTP (GET, POST...). Todos os métodos são gravados.
	Metodo string `json:"metodo"`
	// Caminho da URL, sem a query (ex.: "/orders/42").
	Caminho string `json:"caminho"`
	// Query crua, sem o "?" (ex.: "expand=items&x=1"); "" se não houver.
	Query string `json:"query"`
	// CabecalhosRequisicao enviados pelo cliente, com os sensíveis mascarados.
	// Inclui o Host que o cliente mandou.
	CabecalhosRequisicao http.Header `json:"cabecalhos_requisicao"`
	// CorpoRequisicao (base64 no JSON), até o limite de corpo.
	CorpoRequisicao []byte `json:"corpo_requisicao"`
	// CorpoRequisicaoTruncado é true quando o corpo passou do limite e foi
	// gravado cortado. Omitido quando false.
	CorpoRequisicaoTruncado bool `json:"corpo_requisicao_truncado,omitempty"`
	// Status HTTP devolvido ao cliente.
	Status int `json:"status"`
	// CabecalhosResposta devolvidos ao cliente, com os sensíveis mascarados.
	CabecalhosResposta http.Header `json:"cabecalhos_resposta"`
	// CorpoResposta como chegou ao cliente (base64 no JSON), até o limite.
	// Corpos comprimidos são gravados como vieram.
	CorpoResposta []byte `json:"corpo_resposta"`
	// CorpoRespostaTruncado é true quando o corpo passou do limite.
	CorpoRespostaTruncado bool `json:"corpo_resposta_truncado,omitempty"`
	// LatenciaMs do início da requisição até o fim da resposta, em
	// milissegundos com casas decimais.
	LatenciaMs float64 `json:"latencia_ms"`
}
