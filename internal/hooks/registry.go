package hooks

import (
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

// serviceRegistry implements ServiceRegistry
type serviceRegistry struct {
	messageService message.Service
	sessionService session.Service
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(msgSvc message.Service, sessSvc session.Service) ServiceRegistry {
	return &serviceRegistry{
		messageService: msgSvc,
		sessionService: sessSvc,
	}
}

func (sr *serviceRegistry) MessageService() message.Service {
	return sr.messageService
}

func (sr *serviceRegistry) SessionService() session.Service {
	return sr.sessionService
}
