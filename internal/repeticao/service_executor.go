package repeticao

import (
	"context"
	"sync"

	"github.com/leonardolermen/parallax/internal/captura"
)

type par struct {
	troca           captura.Troca
	base, candidata resposta
	pulada          bool
}

func executar(ctx context.Context, c Config, ts []captura.Troca) ([]par, error) {
	client := novoCliente(c)
	defer client.CloseIdleConnections()
	ps := make([]par, len(ts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := c.Trabalhadores
	if workers > len(ts) {
		workers = len(ts)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				t := ts[idx]
				p := par{troca: t, pulada: pular(t, c)}
				if !p.pulada {
					p.base = enviar(ctx, client, c, c.Base, t)
					if ctx.Err() == nil {
						p.candidata = enviar(ctx, client, c, c.Candidata, t)
					}
				}
				ps[idx] = p
			}
		}()
	}
enviarJobs:
	for i := range ts {
		select {
		case <-ctx.Done():
			break enviarJobs
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	return ps, ctx.Err()
}
