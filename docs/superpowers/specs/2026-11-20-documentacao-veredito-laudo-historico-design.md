# Documentação da parte 3/3 (veredito, laudo e histórico)

Demanda #35, aprovada pelo usuário em 2026-11-20. Só texto; nenhum código muda.

O README já cobre `check`, códigos 0/1/2/4, flags, laudo, `history` e avisos de
segurança. Faltam três ajustes:

1. **README, "O laudo"**: avisar que os nomes de campo das diferenças saem como
   vieram na resposta, sem escape (crase ou quebra de linha estragam o
   Markdown; caractere de controle chega ao terminal). Fonte:
   `gotchas/laudo-chaves-json-cruas.md`.
2. **docs/contrato.md, tabela de códigos**: o código 3 fica reservado; nenhum
   comando o usa desde o `check`. Contrato e `pkg/veredito` não mudam.
3. **README, seção "Decisões"**: só motivos registrados em
   `decisions/parallax-veredito-laudo-historico-demanda-31.md`:
   replay continua com 0 e o portão de CI é o `check`; "reprovado" e
   "inconclusivo" são `regressao` e `revisar`, sem estado novo; zero trocas
   comparadas dá 4; histórico em arquivos JSON, sem banco.

O README também está reservado pela #32: mexer só nessas seções.
