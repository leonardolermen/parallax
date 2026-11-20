# Desenho: testes de veredito, laudo e histórico (#33)

Status: desenho aprovado pelo usuário no Solaris na retomada da demanda #33 ("Aprovo o desenho"). Abordagem A confirmada; plano de execução aguardando revisão.

## Objetivo e abordagens

Ampliar a confiança na implementação já entregue pela demanda #32, cobrindo erros e a integração entre `check` e `history`. A opção A escolhida concentra os testes nesses caminhos. Uma revisão ampla de todos os testes aumentaria o escopo e repetiria cobertura existente; testar somente unidades isoladas deixaria a integração sem comprovação. Essas alternativas não serão adotadas.

## Desenho proposto

- Saída: simular falha do escritor de stdout em Markdown e JSON e falha de gravação de `--report`; conferir código 4, diagnóstico e preservação do histórico já gravado. Cobrir também erros de saída em `history` e no gerador do laudo.
- Entrada e cancelamento: captura vazia, linha inválida depois de uma válida e contexto cancelado devem sair com 4, sem laudo de sucesso nem histórico. Contadores em servidores `httptest` locais comprovarão ausência de tráfego quando a entrada é inválida. Cancelamento durante uma requisição usará sincronização com o handler e prazo limitado, sem depender de pausas arbitrárias.
- Histórico: danificar ou remover individualmente os arquivos de uma execução válida; verificar erros de leitura e a política de listagem existente, incluindo continuidade diante de metadados corrompidos. Não ampliar a validação do formato sem demonstrar um defeito no comportamento contratado.
- Integração: executar `check --json --report`, recuperar a mesma execução com `history show` em JSON e Markdown e comparar o conteúdo completo com os arquivos persistidos. Cobrir aprovado, regressão e revisar por comparação incompleta; validar o contrato JSON e os metadados de captura, alvo e estatísticas.

## Limites e implementação

Preservar o contrato de `pkg/veredito`, os códigos 0/1/2/4 do check, o comportamento 0/4 do replay e os limites de latência aprovados. Usar somente a biblioteca padrão, testes em arquivos novos ao lado dos existentes e helpers já disponíveis quando adequados. Corrigir código de produção apenas se um teste demonstrar defeito dentro deste escopo, reservando os arquivos antes de editar.

Não alterar testes existentes para esconder falhas. Não adicionar dependências, CI ou ajuste de limites com tráfego real. Usar somente rede local com `httptest` e clientes com timeout. Não executar binários nem curl bloqueados neste ambiente; chamar a CLI pelas funções Go existentes.

## Verificação e entrega ao revisor

Rodar `go test -timeout 120s ./...`, com GOCACHE e GOMODCACHE em TEMP, e repetir os testes novos com a mesma suíte e filtro `-run`/`-count=10`. Rodar build e vet. Os testes devem afirmar resultados observáveis, sem comparar valores variáveis entre execuções independentes, como id, horário ou latência.

A suíte de base passou nesta retomada em todos os seis pacotes (resultado do cache). A memória registra que `-race` exige cgo indisponível nesta máquina; permissões Unix 0700/0600 não podem ser verificadas no Windows. Essas limitações serão mantidas explícitas, sem apagar ou pular testes para obter aprovação.

Entregar ao revisor os testes adicionados, eventuais defeitos corrigidos, comandos e resultados, conflitos de reserva e lacunas de ambiente. O Solaris fará os commits; não usar comandos Git de escrita neste worktree.
