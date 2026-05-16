package poll

import (
	"context"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
)

type ScheduledServer struct {
	pluginv1.UnimplementedScheduledTaskServer
	Poller *Poller
}

func (s *ScheduledServer) Run(ctx context.Context, _ *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	if s.Poller == nil {
		return &pluginv1.RunScheduledTaskResponse{}, nil
	}
	_, err := s.Poller.Tick(ctx)
	return &pluginv1.RunScheduledTaskResponse{}, err
}
