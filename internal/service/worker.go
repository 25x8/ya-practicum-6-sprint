package service

import (
	"sync"
	"time"
)

type OrderProcessor struct {
	accrualSvc *AccrualService
	interval   time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewOrderProcessor(accrualSvc *AccrualService) *OrderProcessor {
	return &OrderProcessor{
		accrualSvc: accrualSvc,
		interval:   5 * time.Second, 
		stopCh:     make(chan struct{}),
	}
}

func (p *OrderProcessor) Start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.processLoop()
	}()
}

func (p *OrderProcessor) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

func (p *OrderProcessor) processLoop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.processPendingOrders()
		case <-p.stopCh:
			return
		}
	}
}

func (p *OrderProcessor) processPendingOrders() { /* очередь запросов на получение информации о заказах */}
