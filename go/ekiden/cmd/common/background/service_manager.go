// Package background implements utilities for managing background
// services.
package background

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/oasislabs/ekiden/go/common/logging"
	"github.com/oasislabs/ekiden/go/common/service"
)

// ServiceManager manages a group of background services.
type ServiceManager struct {
	logger *logging.Logger

	services []service.BackgroundService
	termCh   chan service.BackgroundService
	termSvc  service.BackgroundService

	stopCh chan struct{}
}

// Register registers a background service.
func (m *ServiceManager) Register(srv service.BackgroundService) {
	m.services = append(m.services, srv)
	go func() {
		<-srv.Quit()
		select {
		case m.termCh <- srv:
		default:
		}
	}()
}

// RegisterCleanupOnly registers a cleanup only background service.
func (m *ServiceManager) RegisterCleanupOnly(svc service.CleanupAble) {
	m.services = append(m.services, service.NewCleanupOnlyService(svc))
}

// Wait waits for interruption via Stop, SIGINT, SIGTERM, or any of
// the registered services to terminate, and stops all services.
func (m *ServiceManager) Wait() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-m.stopCh:
		m.logger.Info("programatic termination requested")
	case m.termSvc = <-m.termCh:
		m.logger.Info("background task terminated, propagating")
	case <-sigCh:
		m.logger.Info("user requested termination")
	}

	for _, svc := range m.services {
		if svc != m.termSvc {
			svc.Stop()
		}
	}
}

// Stop stops all services.
func (m *ServiceManager) Stop() {
	close(m.stopCh)
}

// Cleanup cleans up after all registered services.
func (m *ServiceManager) Cleanup() {
	m.logger.Debug("begining cleanup")

	for _, svc := range m.services {
		svc.Cleanup()
	}

	m.logger.Debug("finished cleanup")
}

// NewServiceManager creates a new ServiceManager with the provided logger.
func NewServiceManager(logger *logging.Logger) *ServiceManager {
	return &ServiceManager{
		logger: logger,
		termCh: make(chan service.BackgroundService),
		stopCh: make(chan struct{}),
	}
}