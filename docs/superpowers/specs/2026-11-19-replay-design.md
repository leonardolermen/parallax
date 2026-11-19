# Replay e comparação — design aprovado (#24/#25)

Implementar a parte 2/3 em `internal/repeticao`, com CLI em
`cmd/parallax/replay.go`, somente Go 1.22 e biblioteca padrão. A reorganização
da PR #2 substituiu `pkg/motor`; os erros preservam o prefixo `motor:`.

## Abordagem aprovada

Carregar e validar toda a captura e a configuração antes de enviar tráfego.
Um pool limitado (4 trabalhadores por padrão) envia base e candidata em
sequência por troca. Armazenar os pares pelo índice original. Cada requisição
recebe traceparent independente, cliente com timeout e contexto.
Rejeitada a recriação de pkg/motor e adiado o parser YAML.

Somente GET, HEAD e OPTIONS por padrão; outros métodos exigem allow-unsafe.
Pular corpos de requisição truncados. Remover cabeçalhos hop-by-hop, Host,
Content-Length e valores mascarados; permitir injeção de cabeçalhos.

Comparar status, JSON com UseNumber (até 50 diferenças, caminhos $.a[0].b),
corpos não JSON e Content-Type, Location, Cache-Control. Campo removido ou
tipo alterado, inclusive null, quebra contrato. Candidata com 5xx, timeout
ou conexão recusada gera erro_novo; se a base também falha, pre_existente.
Descompactar gzip com leitura limitada. Respostas acima de max-body geram
diferenca de comparação incompleta e a execução continua (resposta do usuário).
Achados incluem resumo de cerca de 500 caracteres e curl da candidata.

Medir latência com platform.Instante/DecorridoMs. Agrupar por método e rota
normalizada (números, UUID, ULID, hex >=16 substituídos por {id}). Nearest-rank
para p50/p95, somente pares com resposta completa dos dois lados. Regressão:
candidata p95 > base p95 * (1 + percentual/100) E delta >= mínimo (50%, 20 ms
por padrão), OU candidata acima do teto absoluto opcional. A regra relativa
vale primeiro: se ela dispara, a origem é sempre branch (decisão do usuário
na revisão #27). A origem é pre_existente só quando a única violação é o teto
e a base também já passava dele.

Saída própria: id, estatisticas (total repetido, identicas, diferentes,
puladas, latencia), achados nunca null. Achados por troca em ordem da captura;
latência ao final. Reutilizar tipos pkg/veredito sem modificar esse pacote.
Replay retorna 0 ao concluir mesmo com achados e 4 em erro. Check segue em 3.
Veredito final, laudo, histórico, YAML e campos ignorados ficam para parte 3.

## Configuração e CLI

Config: Base, Candidata, Trabalhadores=4, Timeout=10s,
PercentualLatencia=50, MinimoLatenciaMs=20, LimiteAbsolutoMs=0,
PermitirInseguros, Cabecalhos http.Header, MaxCorpo=1048576.
Validar URLs http(s) absolutas, números finitos e limites positivos
(teto absoluto zero desativa). Flags obrigatórias capture/base/candidate;
out padrão stdout; workers, timeout, latency-pct, latency-min-ms,
latency-max-ms, allow-unsafe, header repetível, max-body.

## Verificação

Somente httptest local; validar captura inválida com zero tráfego, concorrência
limitada, ordem, traces únicos, limites da latência, tipos de achados, gzip,
truncamento, timeout, conexão recusada e CLI. Sempre go test -timeout 120s ./... .
README e docs/captura.md serão atualizados pelo documentador (#28).
