# Núcleo replay — registro de execução

Implementadas tarefas 1–6 em `internal/repeticao`: configuração, carga/validação completa, cliente HTTP, pool limitado, normalização de endpoint, comparação, percentis/regressão e orquestração pública.

API: `ConfigPadrao() Config`, `(Config).Validar() error`, `Repetir(context.Context, Config, io.Reader) (Resultado,error)`, `Endpoint`, `Percentil`, `Regressao`. Resultado próprio mantém `id`, `estatisticas` (incluindo puladas), `achados` nunca null, tipos reutilizados de `pkg/veredito`.

TDD: baseline `go test -timeout 120s ./...` verde (4 pacotes). Testes escritos antes da implementação, RED confirmado por API ausente. Ciclos adicionais reproduziram RED e foram corrigidos: caminho decodificado com espaço/?/#; recusa de conexão Windows (WSA 10061); status diferente não infere contrato; Diff explícito para headers; hop-by-hop capturado após override Connection; fallback textual inglês refused. Primeiro GREEN integrado incluiu CLI desenvolvida pelo agente principal. Última execução completa verde em todos os 5 pacotes, incluindo CLI e núcleo.

Todas as invocações Go usaram `$env:GOCACHE = Join-Path $env:TEMP 'parallax-task25-go-build'`. Sem instalação, mudança em pkg/veredito ou operações git mutantes.

Cobertura exercitada: validação de configuração, headers e todas as trocas antes de tráfego; captura vazia e inválida; métodos seguros/inseguros e corpo truncado; gzip acima do limite descomprimido; traces independentes com formato W3C; headers removidos/injetados; redirect preservado; timeout 200ms; conexão recusada; cancelamento; concorrência e ordem; tipos JSON com UseNumber e null; quebra de contrato após limite de 50 diferenças; headers explicados no Diff; normalização IDs; nearest-rank e limites exatos; teto preexistente; contadores e JSON achados [].

Revisão própria corrigiu vazamento de hop-by-hop quando Connection era sobrescrito. Incompletos continuam execução e ficam fora de latência. Achados de trocas seguem captura, grupos de latência são ordenados. Captura usa Path já decodificado; construção URL preserva escaping e prefixo do destino.

Limitação conhecida reportada pela revisão independente durante o fechamento: HEAD com Content-Encoding:gzip sem corpo precisa ignorar descompressão. O agente principal está reunindo revisão e coordenará correções. Testes adicionais para detalhes de agregação de latência e fronteira exata do corpo podem ampliar cobertura, sem bloquear os comportamentos já exercitados. Build/vet e revisão independente finais a cargo do agente principal.

## Correções da revisão independente — onda única

Os três P2 de `review-report.md` foram reproduzidos em testes antes de alterar produção, com `go test -timeout 120s ./...` retornando RED: `TestGzipSemCorpoPermitido` (HEAD/204/304 incompletos e P95 zero), `TestRegressaoPrioridadeTeto` (60/100/teto50 retornava branch), `TestPortasInvalidasZeroTrafego` (portas fora de faixa e vazias aceitas, tráfego atingia servidor válido).

Correções limitadas: cliente ignora inicialização gzip em HEAD/204/304; regressão resolve teto absoluto antes de relativo; configuração valida portas explícitas entre 1 e 65535 e rejeita porta vazia nos dois destinos. Sintaxe inválida continua rejeitada pelo parser. Testes preservam URLs IPv6 sem/com porta, HTTP/HTTPS padrão e porta 65535. Todas as portas inválidas agora retornam erro antes de tráfego, com contador zero.

Após as correções, `go test -timeout 120s ./...` GREEN nos cinco pacotes (CLI 1.140s; núcleo 1.423s). Adicionado, a pedido do coordenador, teste integrado da latência usando teto 1e-9ms e rotas /x/1,/x/2,/outro: dois grupos, ordem determinística, achados Latencia pre_existente e percentis positivos, sem sleeps. Verificação final `go test -timeout 120s ./...` GREEN: cmd/parallax cached; captura cached; platform cached; repeticao 1.403s; veredito cached.

Comandos Go desta onda usaram GOCACHE=`Join-Path $env:TEMP 'parallax-task25-go-build'` e GOMODCACHE=`Join-Path $env:TEMP 'parallax-task25-go-mod'`. Arquivos alterados nesta onda: service_cliente.go, service_config.go, service_latencia.go e novo service_revisao_test.go, além deste registro. A pendência HEAD anteriormente registrada foi resolvida. Núcleo novamente congelado para re-review focada.
